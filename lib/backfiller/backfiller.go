package backfiller

import (
	"context"
	"fmt"
	"github.com/kadaan/promutil/config"
	"github.com/kadaan/promutil/lib/block"
	"github.com/kadaan/promutil/lib/database"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
	"os"
	"regexp"
	"runtime"
	"sort"
	"sync"
	"time"
)

const (
	tmpBackfillDirSuffix = ".tmp-for-backfill"
)

var (
	MaxParallelism = uint8(runtime.NumCPU())
)

type backfiller struct {
	config *backfillConfig
}

type Backfiller interface {
	Backfill() error
}

type backfillConfig struct {
	OutputDirectory           string
	StartTime                 time.Time
	EndTime                   time.Time
	SampleInterval            time.Duration
	RecordingRules            config.RecordingRules
	RecordingRuleGroupFilters []*regexp.Regexp
	RecordingRuleNameFilters  []*regexp.Regexp
	Parallelism               uint8
}

func Backfill(c *config.BackfillConfig) error {
	var parallelism uint8 = 1
	if c.Parallelism > 0 {
		if c.Parallelism <= MaxParallelism {
			parallelism = c.Parallelism
		} else {
			parallelism = MaxParallelism
		}
	}
	var recordingRules config.RecordingRules
	for _, r := range c.RuleConfig {
		if shouldIncludeRecordingRule(c, r) {
			recordingRules = append(recordingRules, r)
		}
	}
	if len(recordingRules) == 0 {
		return fmt.Errorf("no recording rules left after filtering")
	}

	cfg := &backfillConfig{
		OutputDirectory: c.OutputDirectory,
		StartTime:       c.Start,
		EndTime:         c.End,
		SampleInterval:  c.SampleInterval,
		RecordingRules:  recordingRules,
		Parallelism:     parallelism,
	}
	b := backfiller{cfg}
	err := b.Backfill()
	if err != nil {
		return errors.Wrap(err, "failed to backfill")
	}
	return nil
}

func shouldIncludeRecordingRule(c *config.BackfillConfig, recordingRule *config.RecordingRule) bool {
	return evaluateFilters(recordingRule.Group, c.RuleGroupFilters) &&
		evaluateFilters(recordingRule.Name(), c.RuleNameFilters)
}

func evaluateFilters(value string, filters []*regexp.Regexp) bool {
	for _, f := range filters {
		if f.MatchString(value) {
			return true
		}
	}
	return false
}

func (b *backfiller) Backfill() error {
	qdb, err := database.NewDatabase(b.config.OutputDirectory, database.DefaultBlockDuration, database.DefaultRetention,
		context.Background())
	if err != nil {
		return errors.Wrap(err, "failed to open query db")
	}
	defer func(db database.Database) {
		_ = db.Close()
	}(qdb)

	queryManager, err := qdb.QueryManager()
	if err != nil {
		return errors.Wrap(err, "failed to get query manager")
	}

	startInMs := b.config.StartTime.Unix() * int64(time.Second/time.Millisecond)
	endInMs := b.config.EndTime.Unix() * int64(time.Second/time.Millisecond)
	blockDuration := database.GetCompatibleBlockDuration(endInMs - startInMs)

	tempDirectory, err := database.NewTempDirectory(b.config.OutputDirectory, tmpBackfillDirSuffix)
	if err != nil {
		return err
	}

	db, err := database.NewDatabase(tempDirectory, blockDuration, database.DefaultRetention,
		context.Background())
	if err != nil {
		return errors.Wrap(err, "failed to open db")
	}
	defer func(db database.Database) {
		_ = db.Close()
	}(db)

	appendManager, err := db.AppendManager()
	if err != nil {
		return errors.Wrap(err, "failed to get appender")
	}

	var wg sync.WaitGroup
	var cg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	s := newCanceller()
	errChan := make(chan error)
	inputChan := make(chan block.PlanEntry[config.RecordingRule])
	producer, err := newPlanProducer(b.config, database.DefaultBlockDuration, &wg, ctx, inputChan)
	if err != nil {
		cancel()
		return errors.Wrap(err, "failed to start producer")
	}
	go producer.run()

	for i := uint8(0); i < b.config.Parallelism; i++ {
		cg.Add(1)
		consumer, errC := newPlanConsumer(i, appendManager, queryManager, &cg, &wg, ctx, errChan, inputChan, s)
		if errC != nil {
			cancel()
			return errors.Wrap(errC, "failed to start consumers")
		}
		go consumer.run()
	}

	go func() {
		select {
		case <-s.C:
			_, _ = fmt.Fprintf(os.Stderr, "Stopping producer, consumers, and db\n")
			cancel()
		}
	}()

	cg.Wait()
	cancel()

	err = appendManager.Close()
	if err != nil {
		return err
	}

	err = db.Compact()
	if err != nil {
		return err
	}

	return database.MoveBlocks(tempDirectory, b.config.OutputDirectory)
}

type canceller struct {
	C    chan struct{}
	once sync.Once
}

func newCanceller() *canceller {
	return &canceller{
		C: make(chan struct{}),
	}
}

func (s *canceller) Cancel() {
	s.once.Do(func() {
		close(s.C)
	})
}

type planEntry struct {
	start         int64
	end           int64
	step          int64
	recordingRule *config.RecordingRule
}

type planProducer struct {
	startTime      time.Time
	endTime        time.Time
	sampleInterval time.Duration
	recordingRules config.RecordingRules
	blockDuration  int64
	wg             *sync.WaitGroup
	ctx            context.Context
	output         chan<- block.PlanEntry[config.RecordingRule]
	planner        block.Planner[config.RecordingRule]
}

func newPlanProducer(cfg *backfillConfig, blockDuration int64, wg *sync.WaitGroup, ctx context.Context, output chan<- block.PlanEntry[config.RecordingRule]) (*planProducer, error) {
	return &planProducer{
		planner:        block.NewPlanner[config.RecordingRule](cfg.StartTime, cfg.EndTime, blockDuration, cfg.SampleInterval),
		recordingRules: cfg.RecordingRules,
		wg:             wg,
		ctx:            ctx,
		output:         output,
	}, nil
}

func (p *planProducer) run() {
	defer close(p.output)
	for _, plan := range p.planner.Plan(p.expandPlan) {
		p.wg.Add(len(plan))
		for _, e := range plan {
			select {
			case <-p.ctx.Done():
				_, _ = fmt.Fprintf(os.Stderr, "Cancelling producer\n")
				return
			case p.output <- e:
			}
		}

		if !p.wait() {
			return
		}
	}
	_, _ = fmt.Fprintf(os.Stderr, "Stopping producer\n")
}

func (p *planProducer) expandPlan(chunkStart int64, chunkEnd int64, stepDuration int64) []block.PlanEntry[config.RecordingRule] {
	var planEntries []block.PlanEntry[config.RecordingRule]
	for _, recordingRule := range p.recordingRules {
		planEntries = append(planEntries, block.NewPlanEntry(chunkStart, chunkEnd, stepDuration, recordingRule))
	}
	return planEntries
}

func (p *planProducer) wait() bool {
	c := make(chan struct{})
	go func() {
		defer close(c)
		p.wg.Wait()
	}()
	select {
	case <-p.ctx.Done():
		_, _ = fmt.Fprintf(os.Stderr, "Cancelling producer\n")
		return false
	case <-c:
		return true
	}
}

func (p *planProducer) planBlock(blockStart time.Time, blockEnd time.Time, stepDuration int64) []planEntry {
	var plan []planEntry
	start := blockStart.UnixNano() / int64(time.Millisecond/time.Nanosecond)
	end := blockEnd.UnixNano() / int64(time.Millisecond/time.Nanosecond)
	chunkDuration := (end - start) / 4
	chunkStart := start
	for ; chunkStart <= end; chunkStart = chunkStart + chunkDuration {
		chunkEnd := chunkStart + chunkDuration - 1
		if chunkEnd > end {
			break
		}
		for _, recordingRule := range p.recordingRules {
			plan = append(plan, planEntry{
				chunkStart,
				chunkEnd,
				stepDuration,
				recordingRule,
			})
		}
	}
	return plan
}

type planConsumer struct {
	name      string
	querier   database.Querier
	appender  database.Appender
	cg        *sync.WaitGroup
	wg        *sync.WaitGroup
	ctx       context.Context
	errorChan chan<- error
	input     <-chan block.PlanEntry[config.RecordingRule]
	s         *canceller
	stopOnce  sync.Once
}

func newPlanConsumer(id uint8, appendManager database.AppendManager, queryManager database.QueryManager, cg *sync.WaitGroup, wg *sync.WaitGroup, ctx context.Context, errorChan chan<- error, input <-chan block.PlanEntry[config.RecordingRule], s *canceller) (*planConsumer, error) {
	appender, err := appendManager.NewAppender()
	if err != nil {
		return nil, err
	}
	name := fmt.Sprintf("backfill%d", id)
	return &planConsumer{
		name:      name,
		querier:   queryManager.NewQuerier(),
		appender:  appender,
		cg:        cg,
		wg:        wg,
		ctx:       ctx,
		errorChan: errorChan,
		input:     input,
		s:         s,
	}, nil
}

func (p *planConsumer) run() {
	for {
		select {
		case <-p.ctx.Done():
			p.stopOnce.Do(func() {
				p.printMessage("Cancelling consumer")
				p.cg.Done()
			})
			return
		case plan, ok := <-p.input:
			if !ok {
				p.stopOnce.Do(func() {
					p.printMessage("Stopping consumer")
					p.cg.Done()
				})
				return
			}
			err := p.executePlan(plan)
			if err != nil {
				p.stopOnce.Do(func() {
					p.printMessage("Cancelling consumer")
					p.s.Cancel()
					p.cg.Done()
				})
				return
			} else {
				p.wg.Done()
			}
		}
	}
}

func (p *planConsumer) executePlan(plan block.PlanEntry[config.RecordingRule]) error {
	p.printMessage(fmt.Sprintf("Backfilling %s", p.planDescription(plan)))
	res, err := p.querier.QueryRangeRule(p.ctx, plan.Data(), plan.Start(), plan.End(), plan.Step())
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to run recording rule '%s'", plan.Data().Name))
	}
	for {
		if ok, sample := res.Next(); !ok {
			break
		} else {
			err = p.appender.Add(sample)
			if err != nil {
				switch err.Error() {
				case storage.ErrOutOfOrderSample.Error():
				case storage.ErrOutOfBounds.Error():
				case storage.ErrDuplicateSampleForTimestamp.Error():
					continue
				default:
					return p.handleExecutePlanError(plan, "could write data", err)
				}
			}
		}
	}
	return nil
}

func (p *planConsumer) printMessage(msg string) {
	_, _ = fmt.Fprintf(os.Stderr, "%s\n", msg)
}

func (p *planConsumer) handleExecutePlanError(plan block.PlanEntry[config.RecordingRule], msg string, err error) error {
	p.printMessage(fmt.Sprintf("Failed to migrate %s [%s]: %v", p.planDescription(plan), msg, err))
	return err
}

func (p *planConsumer) planDescription(plan block.PlanEntry[config.RecordingRule]) string {
	return fmt.Sprintf("'%s' from %s", plan.Data().Name(), formatDateRange(plan.Start(), plan.End()))
}

func (p *planConsumer) labelProtosToLabels(labelPairs []prompb.Label) labels.Labels {
	result := make(labels.Labels, 0, len(labelPairs))
	for _, l := range labelPairs {
		result = append(result, labels.Label{
			Name:  l.Name,
			Value: l.Value,
		})
	}
	sort.Sort(result)
	return result
}

func formatDateRange(start int64, end int64) string {
	return fmt.Sprintf("%s to %s", formatDate(start), formatDate(end))
}

func formatDate(value int64) string {
	return time.UnixMilli(value).Format("2006-01-02T15:04:05")
}

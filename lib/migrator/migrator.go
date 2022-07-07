package migrator

import (
	"context"
	"fmt"
	"github.com/cenkalti/backoff"
	"github.com/kadaan/promutil/config"
	"github.com/kadaan/promutil/lib/block"
	"github.com/kadaan/promutil/lib/database"
	"github.com/pkg/errors"
	promConfig "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"net/url"
	"os"
	"runtime"
	"sort"
	"sync"
	"time"
)

const (
	tmpMigrateDirSuffix = ".tmp-for-migrate"
)

var (
	MaxParallelism = uint8(runtime.NumCPU())
)

type migrator struct {
	config *migrateConfig
}

type Migrator interface {
	Migrate() error
}

type migrateConfig struct {
	OutputDirectory string
	Scheme          string
	Host            string
	Port            uint16
	StartTime       time.Time
	EndTime         time.Time
	SampleInterval  time.Duration
	MatcherSets     map[string][]*labels.Matcher
	Parallelism     uint8
}

func Migrate(c *config.MigrateConfig) error {
	var parallelism uint8 = 1
	if c.Parallelism > 0 {
		if c.Parallelism <= MaxParallelism {
			parallelism = c.Parallelism
		} else {
			parallelism = MaxParallelism
		}
	}

	var matcherSets = make(map[string][]*labels.Matcher)
	for _, s := range c.MatcherSetExpressions {
		matchers, err := parser.ParseMetricSelector(s)
		if err != nil {
			return errors.Wrap(err, "failed to parse matcher")
		}
		matcherSets[s] = matchers
	}

	cfg := &migrateConfig{
		OutputDirectory: c.OutputDirectory,
		Scheme:          string(c.Scheme),
		Host:            c.Host,
		Port:            c.Port,
		StartTime:       c.Start,
		EndTime:         c.End,
		SampleInterval:  c.SampleInterval,
		MatcherSets:     matcherSets,
		Parallelism:     parallelism,
	}
	migrator := migrator{cfg}
	err := migrator.Migrate()
	if err != nil {
		return errors.Wrap(err, "failed to migrate")
	}
	return nil
}

func (m *migrator) Migrate() error {
	promUrl, err := url.Parse(fmt.Sprintf("%s://%s:%d/api/v1/read", m.config.Scheme, m.config.Host, m.config.Port))
	if err != nil {
		return errors.Wrap(err, "failed to parse promUrl")
	}

	clientConfig := &remote.ClientConfig{
		URL:              &promConfig.URL{URL: promUrl},
		Timeout:          model.Duration(2 * time.Minute),
		HTTPClientConfig: promConfig.HTTPClientConfig{},
		RetryOnRateLimit: true,
	}

	startInMs := m.config.StartTime.Unix() * int64(time.Second/time.Millisecond)
	endInMs := m.config.EndTime.Unix() * int64(time.Second/time.Millisecond)
	blockDuration := database.GetCompatibleBlockDuration(endInMs - startInMs)

	tempDirectory, err := database.NewTempDirectory(m.config.OutputDirectory, tmpMigrateDirSuffix)
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
	inputChan := make(chan block.PlanEntry[planData])

	producer, err := newPlanProducer(m.config, database.DefaultBlockDuration, &wg, ctx, inputChan)
	if err != nil {
		cancel()
		return errors.Wrap(err, "failed to start producer")
	}
	go producer.run()

	for i := uint8(0); i < m.config.Parallelism; i++ {
		cg.Add(1)
		consumer, errC := newPlanConsumer(i, appendManager, clientConfig, &cg, &wg, ctx, errChan, inputChan, s)
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

	return database.MoveBlocks(tempDirectory, m.config.OutputDirectory)
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

type planData struct {
	expression string
	matcher    []*labels.Matcher
}

type planProducer struct {
	planner     block.Planner[planData]
	matcherSets map[string][]*labels.Matcher
	wg          *sync.WaitGroup
	ctx         context.Context
	output      chan<- block.PlanEntry[planData]
}

func newPlanProducer(cfg *migrateConfig, blockDuration int64, wg *sync.WaitGroup, ctx context.Context, output chan<- block.PlanEntry[planData]) (*planProducer, error) {
	return &planProducer{
		planner:     block.NewPlanner[planData](cfg.StartTime, cfg.EndTime, blockDuration, cfg.SampleInterval),
		matcherSets: cfg.MatcherSets,
		wg:          wg,
		ctx:         ctx,
		output:      output,
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

func (p *planProducer) expandPlan(chunkStart int64, chunkEnd int64, stepDuration int64) []block.PlanEntry[planData] {
	var planEntries []block.PlanEntry[planData]
	for expression, matcher := range p.matcherSets {
		d := &planData{
			expression: expression,
			matcher:    matcher,
		}
		planEntries = append(planEntries, block.NewPlanEntry(chunkStart, chunkEnd, stepDuration, d))
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

type planConsumer struct {
	name      string
	client    remote.ReadClient
	appender  database.Appender
	cg        *sync.WaitGroup
	wg        *sync.WaitGroup
	ctx       context.Context
	errorChan chan<- error
	input     <-chan block.PlanEntry[planData]
	s         *canceller
	stopOnce  sync.Once
}

func newPlanConsumer(id uint8, appendManager database.AppendManager, clientConfig *remote.ClientConfig, cg *sync.WaitGroup, wg *sync.WaitGroup, ctx context.Context, errorChan chan<- error, input <-chan block.PlanEntry[planData], s *canceller) (*planConsumer, error) {
	appender, err := appendManager.NewAppender()
	if err != nil {
		return nil, err
	}
	name := fmt.Sprintf("migrate%d", id)
	client, err := remote.NewReadClient(name, clientConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create remote client")
	}
	return &planConsumer{
		name:      name,
		client:    client,
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

func (p *planConsumer) executePlan(plan block.PlanEntry[planData]) error {
	hints := &storage.SelectHints{
		Start: plan.Start(),
		End:   plan.End(),
		Step:  plan.Step(),
		Range: 0,
		Func:  "",
	}
	p.printMessage(fmt.Sprintf("Migrating %s", p.planDescription(plan)))
	query, err := remote.ToQuery(hints.Start, hints.End, plan.Data().matcher, hints)
	if err != nil {
		return p.handleExecutePlanError(plan, "could not create query", err)
	}

	var res *prompb.QueryResult
	err = backoff.Retry(func() error {
		r, e := p.client.Read(context.Background(), query)
		if e != nil {
			return e
		}
		res = r
		return nil
	}, backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 5))
	if err != nil {
		return p.handleExecutePlanError(plan, "could not read data", err)
	}
	sample := &promql.Sample{}
	for _, ts := range res.Timeseries {
		sample.Metric = p.labelProtosToLabels(ts.Labels)
		for _, s := range ts.Samples {
			if value.IsStaleNaN(s.Value) || s.Timestamp < plan.Start() {
				continue
			}
			sample.T = s.Timestamp
			sample.V = s.Value
			err = p.appender.Add(sample)
			if err != nil {
				switch err.Error() {
				case storage.ErrOutOfOrderSample.Error():
				case storage.ErrOutOfBounds.Error():
					break
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

func (p *planConsumer) handleExecutePlanError(plan block.PlanEntry[planData], msg string, err error) error {
	p.printMessage(fmt.Sprintf("Failed to migrate %s [%s]: %v", p.planDescription(plan), msg, err))
	return err
}

func (p *planConsumer) planDescription(plan block.PlanEntry[planData]) string {
	return fmt.Sprintf("'%s' from %s", plan.Data().expression, formatDateRange(plan.Start(), plan.End()))
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

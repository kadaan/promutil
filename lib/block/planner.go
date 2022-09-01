package block

import (
	"context"
	"fmt"
	"github.com/kadaan/promutil/lib/common"
	"github.com/kadaan/promutil/lib/database"
	"github.com/kadaan/promutil/lib/errors"
	"k8s.io/klog/v2"
	"runtime"
	"sync"
	"time"
)

const (
	tmpGenerateDirSuffix = ".tmp-for-generate"
)

var (
	MaxParallelism = uint8(runtime.NumCPU())
)

type PlanEntry[V fmt.Stringer] interface {
	Start() int64
	End() int64
	Step() int64
	Data() *V
}

func NewPlanEntry[V fmt.Stringer](name string, start int64, end int64, step int64, data *V) PlanEntry[V] {
	return &planEntry[V]{
		name:  name,
		start: start,
		end:   end,
		step:  step,
		data:  data,
	}
}

type planEntry[V fmt.Stringer] struct {
	name  string
	start int64
	end   int64
	step  int64
	data  *V
}

func (p *planEntry[V]) Start() int64 {
	return p.start
}

func (p *planEntry[V]) End() int64 {
	return p.end
}

func (p *planEntry[V]) Step() int64 {
	return p.step
}

func (p *planEntry[V]) Data() *V {
	return p.data
}

func (p *planEntry[V]) String() string {
	return fmt.Sprintf("%s for '%s' from %s", p.name, *p.Data(), common.FormatDateRange(p.Start(), p.End()))
}

type PlannerConfig interface {
	OutputDirectory() string
	StartTime() time.Time
	EndTime() time.Time
	BlockDuration() int64
	SampleInterval() time.Duration
	Parallelism() uint8
}

func NewPlannerConfig(outputDirectory string, startTime time.Time, endTime time.Time, sampleInterval time.Duration, parallelism int) PlannerConfig {
	var prl uint8 = 1
	if parallelism > 0 {
		if parallelism <= int(MaxParallelism) {
			prl = uint8(parallelism)
		} else {
			prl = MaxParallelism
		}
	}
	return &plannerConfig{
		outputDirectory: outputDirectory,
		startTime:       startTime,
		endTime:         endTime,
		sampleInterval:  sampleInterval,
		parallelism:     prl,
	}
}

type plannerConfig struct {
	outputDirectory string
	startTime       time.Time
	endTime         time.Time
	sampleInterval  time.Duration
	parallelism     uint8
}

func (c plannerConfig) OutputDirectory() string {
	return c.outputDirectory
}

func (c plannerConfig) StartTime() time.Time {
	return c.startTime
}

func (c plannerConfig) EndTime() time.Time {
	return c.endTime
}

func (c plannerConfig) BlockDuration() int64 {
	return database.DefaultBlockDuration
}

func (c plannerConfig) SampleInterval() time.Duration {
	return c.sampleInterval
}

func (c plannerConfig) Parallelism() uint8 {
	return c.parallelism
}

type Planner[V fmt.Stringer] interface {
	Plan(transform func(int64, int64, int64) []PlanEntry[V]) [][]PlanEntry[V]
}

func NewPlanner[V fmt.Stringer](config PlannerConfig) Planner[V] {
	return &planner[V]{
		config: config,
	}
}

type planner[V fmt.Stringer] struct {
	config PlannerConfig
}

func (p *planner[V]) Plan(transform func(int64, int64, int64) []PlanEntry[V]) [][]PlanEntry[V] {
	var results [][]PlanEntry[V]
	startInMs := p.config.StartTime().Unix() * int64(time.Second/time.Millisecond)
	endInMs := p.config.EndTime().Unix() * int64(time.Second/time.Millisecond)
	blockStart := p.config.BlockDuration() * (startInMs / p.config.BlockDuration())
	stepDuration := int64(p.config.SampleInterval() / (time.Millisecond / time.Nanosecond))
	for ; blockStart <= endInMs; blockStart = blockStart + p.config.BlockDuration() {
		blockEnd := blockStart + p.config.BlockDuration() - 1
		currStart := common.MaxInt64(blockStart/int64(time.Second/time.Millisecond), p.config.StartTime().Unix())
		startWithAlignment := p.evalTimestamp(time.Unix(currStart, 0).UTC().UnixNano(), stepDuration)
		for startWithAlignment.Unix() < currStart {
			startWithAlignment = startWithAlignment.Add(p.config.SampleInterval())
		}
		end := time.Unix(common.MinInt64(blockEnd/int64(time.Second/time.Millisecond), p.config.EndTime().Unix()), 0).UTC()
		if end.Equal(startWithAlignment) || end.Before(startWithAlignment) {
			break
		}

		blockPlan := p.planBlock(startWithAlignment, end, stepDuration, transform)
		results = append(results, blockPlan)
	}
	return results
}

func (p *planner[V]) planBlock(blockStart time.Time, blockEnd time.Time, stepDuration int64, transform func(int64, int64, int64) []PlanEntry[V]) []PlanEntry[V] {
	var plan []PlanEntry[V]
	start := blockStart.UnixNano() / int64(time.Millisecond/time.Nanosecond)
	end := blockEnd.UnixNano() / int64(time.Millisecond/time.Nanosecond)
	chunkDuration := (end - start) / 4

	chunkStart := start
	for ; chunkStart <= end; chunkStart = chunkStart + chunkDuration {
		chunkEnd := chunkStart + chunkDuration - 1
		if chunkEnd > end {
			break
		}
		for _, pe := range transform(chunkStart, chunkEnd, stepDuration) {
			plan = append(plan, pe)
		}
	}
	return plan
}

func (p *planner[V]) evalTimestamp(startTime int64, stepDuration int64) time.Time {
	var (
		offset = stepDuration
		adjNow = startTime - stepDuration
		base   = adjNow - (adjNow % stepDuration)
	)

	return time.Unix(0, base+offset).UTC()
}

type PlanGenerator[V fmt.Stringer] interface {
	Generate(chunkStart int64, chunkEnd int64, stepDuration int64) []PlanEntry[V]
}

type PlanProducer interface {
	Run()
}

type planProducer[V fmt.Stringer] struct {
	planner   Planner[V]
	generator PlanGenerator[V]
	wg        *sync.WaitGroup
	ctx       context.Context
	output    chan<- PlanEntry[V]
}

func NewPlanProducer[V fmt.Stringer](cfg PlannerConfig, wg *sync.WaitGroup, ctx context.Context, generator PlanGenerator[V], output chan<- PlanEntry[V]) (PlanProducer, error) {
	return &planProducer[V]{
		planner:   NewPlanner[V](cfg),
		generator: generator,
		wg:        wg,
		ctx:       ctx,
		output:    output,
	}, nil
}

func (p *planProducer[V]) Run() {
	defer close(p.output)
	for _, plan := range p.planner.Plan(p.generator.Generate) {
		p.wg.Add(len(plan))
		for _, e := range plan {
			select {
			case <-p.ctx.Done():
				klog.Error("Cancelling producer")
				return
			case p.output <- e:
			}
		}

		if !p.wait() {
			return
		}
	}
	klog.V(0).Infof("Stopping producer")
}

func (p *planProducer[V]) wait() bool {
	c := make(chan struct{})
	go func() {
		defer close(c)
		p.wg.Wait()
	}()
	select {
	case <-p.ctx.Done():
		klog.V(0).Infof("Cancelling producer")
		return false
	case <-c:
		return true
	}
}

type PlanExecutor[V fmt.Stringer] interface {
	Execute(ctx context.Context, logger PlanLogger[V], plan PlanEntry[V]) error
}

type PlanConsumer interface {
	Run()
}

type planConsumer[V fmt.Stringer] struct {
	name      string
	l         PlanLogger[V]
	e         PlanExecutor[V]
	cg        *sync.WaitGroup
	wg        *sync.WaitGroup
	ctx       context.Context
	errorChan chan<- error
	input     <-chan PlanEntry[V]
	s         common.Canceller
	stopOnce  sync.Once
}

func NewPlanConsumer[V fmt.Stringer](name string, cg *sync.WaitGroup, wg *sync.WaitGroup, ctx context.Context, errorChan chan<- error, input <-chan PlanEntry[V], s common.Canceller, e PlanExecutor[V]) (PlanConsumer, error) {
	logger := &planLogger[V]{}
	return &planConsumer[V]{
		name:      name,
		l:         logger,
		e:         e,
		cg:        cg,
		wg:        wg,
		ctx:       ctx,
		errorChan: errorChan,
		input:     input,
		s:         s,
	}, nil
}

func (p *planConsumer[V]) Run() {
	for {
		select {
		case <-p.ctx.Done():
			p.stopOnce.Do(func() {
				p.l.PrintMessage("Cancelling consumer")
				p.cg.Done()
			})
			return
		case plan, ok := <-p.input:
			if !ok {
				p.stopOnce.Do(func() {
					p.l.PrintMessage("Stopping consumer")
					p.cg.Done()
				})
				return
			}
			p.l.PrintMessage(fmt.Sprintf("Running %v", plan))
			err := p.e.Execute(p.ctx, p.l, plan)
			if err != nil {
				p.l.PrintExecutePlanError(plan, "could write data", err)
				p.stopOnce.Do(func() {
					p.l.PrintMessage("Cancelling consumer")
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

type PlanLogger[V fmt.Stringer] interface {
	PrintMessage(format string, args ...interface{})
	PrintExecutePlanError(plan PlanEntry[V], msg string, err error)
}

type planLogger[V fmt.Stringer] struct {
}

func (p *planLogger[V]) PrintMessage(format string, args ...interface{}) {
	klog.V(0).Infof(format, args...)
}

func (p *planLogger[V]) PrintExecutePlanError(plan PlanEntry[V], msg string, err error) {
	p.PrintMessage("Failed to %v [%s]: %v", plan, msg, err)
}

type PlanExecutorCreator[V fmt.Stringer] interface {
	Create(name string, appender database.Appender) (PlanExecutor[V], error)
}

type PlannedBlockWriter interface {
	Run() error
}

func NewPlannedBlockWriter[V fmt.Stringer](config PlannerConfig, generator PlanGenerator[V], executorCreator PlanExecutorCreator[V]) PlannedBlockWriter {
	return &plannedBlockWriter[V]{
		config:          config,
		generator:       generator,
		executorCreator: executorCreator,
	}
}

type plannedBlockWriter[V fmt.Stringer] struct {
	config          PlannerConfig
	generator       PlanGenerator[V]
	executorCreator PlanExecutorCreator[V]
}

func (p *plannedBlockWriter[V]) Run() error {
	startInMs := p.config.StartTime().Unix() * int64(time.Second/time.Millisecond)
	endInMs := p.config.EndTime().Unix() * int64(time.Second/time.Millisecond)
	blockDuration := database.GetCompatibleBlockDuration(endInMs - startInMs)

	tempDirectory, err := database.NewTempDirectory(p.config.OutputDirectory(), tmpGenerateDirSuffix)
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
	s := common.NewCanceller()
	errChan := make(chan error)
	inputChan := make(chan PlanEntry[V])

	producer, err := NewPlanProducer[V](p.config, &wg, ctx, p.generator, inputChan)
	if err != nil {
		cancel()
		return errors.Wrap(err, "failed to start producer")
	}
	go producer.Run()

	for i := uint8(0); i < p.config.Parallelism(); i++ {
		cg.Add(1)

		appender, errA := appendManager.NewAppender()
		if errA != nil {
			cancel()
			return errors.Wrap(errA, "failed to start consumers")
		}

		name := fmt.Sprintf("planConsumer%d", i)
		executor, errE := p.executorCreator.Create(name, appender)
		if errE != nil {
			cancel()
			return errors.Wrap(errE, "failed to start consumers")
		}
		consumer, errC := NewPlanConsumer(name, &cg, &wg, ctx, errChan, inputChan, s, executor)
		if errC != nil {
			cancel()
			return errors.Wrap(errC, "failed to start consumers")
		}
		go consumer.Run()
	}

	go func() {
		select {
		case <-s.C():
			klog.V(0).Infof("Stopping producer, consumers, and db")
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

	return database.MoveBlocks(tempDirectory, p.config.OutputDirectory())
}

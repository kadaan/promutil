package backfiller

import (
	"context"
	"github.com/kadaan/promutil/config"
	"github.com/kadaan/promutil/lib/block"
	"github.com/kadaan/promutil/lib/command"
	"github.com/kadaan/promutil/lib/database"
	"github.com/kadaan/promutil/lib/errors"
	"regexp"
)

func NewBackfiller() command.Task[config.BackfillConfig] {
	return &backfiller{}
}

type backfiller struct {
}

func (t *backfiller) Run(c *config.BackfillConfig) error {
	var recordingRules config.RecordingRules
	for _, r := range c.RuleConfig {
		if shouldIncludeRecordingRule(c, r) {
			recordingRules = append(recordingRules, r)
		}
	}
	if len(recordingRules) == 0 {
		return errors.New("no recording rules left after filtering")
	}

	qdb, err := database.NewDatabase(c.Directory, database.DefaultBlockDuration, database.DefaultRetention,
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

	plannerConfig := block.NewPlannerConfig(c.Directory, c.Start, c.End, c.SampleInterval, int(c.Parallelism))
	generator := &planGenerator{recordingRules: recordingRules}
	executorCreator := &planExecutorCreator{queryManager: queryManager}
	writer := block.NewPlannedBlockWriter[config.RecordingRule](plannerConfig, generator, executorCreator)
	return writer.Run()
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

type planGenerator struct {
	recordingRules config.RecordingRules
}

func (p *planGenerator) Generate(chunkStart int64, chunkEnd int64, stepDuration int64) []block.PlanEntry[config.RecordingRule] {
	var planEntries []block.PlanEntry[config.RecordingRule]
	for _, recordingRule := range p.recordingRules {
		planEntries = append(planEntries, block.NewPlanEntry("backfill", chunkStart, chunkEnd, stepDuration, recordingRule))
	}
	return planEntries
}

type planExecutorCreator struct {
	queryManager database.QueryManager
}

func (p *planExecutorCreator) Create(_ string, appender database.Appender) (block.PlanExecutor[config.RecordingRule], error) {
	return &planExecutor{
		querier:  p.queryManager.NewQuerier(),
		appender: appender,
	}, nil
}

type planExecutor struct {
	querier  database.Querier
	appender database.Appender
}

func (p *planExecutor) Execute(ctx context.Context, _ block.PlanLogger[config.RecordingRule], plan block.PlanEntry[config.RecordingRule]) error {
	res, err := p.querier.QueryRangeRule(ctx, plan.Data(), plan.Start(), plan.End(), plan.Step())
	if err != nil {
		return errors.Wrap(err, "failed to run recording rule '%s'", plan.Data().Name)
	}
	for {
		if ok, sample := res.Next(); !ok {
			break
		} else {
			if err = p.appender.Add(sample); err != nil {
				return errors.Wrap(err, "failed to add sample: %s", sample)
			}
		}
	}
	return nil
}

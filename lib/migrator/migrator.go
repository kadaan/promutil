package migrator

import (
	"context"
	"github.com/cenkalti/backoff"
	"github.com/kadaan/promutil/config"
	"github.com/kadaan/promutil/lib/block"
	"github.com/kadaan/promutil/lib/command"
	"github.com/kadaan/promutil/lib/common"
	"github.com/kadaan/promutil/lib/database"
	"github.com/kadaan/promutil/lib/errors"
	promConfig "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"time"
)

const (
	maxQueryRetryAttempts = 5
)

func NewMigrator() command.Task[config.MigrateConfig] {
	return &migrator{}
}

type migrator struct {
}

func (t *migrator) Run(c *config.MigrateConfig) error {
	clientConfig := &remote.ClientConfig{
		URL:              &promConfig.URL{URL: c.Host},
		Timeout:          model.Duration(2 * time.Minute),
		HTTPClientConfig: promConfig.HTTPClientConfig{},
		RetryOnRateLimit: true,
	}

	plannerConfig := block.NewPlannerConfig(c.Directory, c.Start, c.End, c.SampleInterval, int(c.Parallelism))
	generator := &planGenerator{matcherSets: c.Matchers}
	executorCreator := &planExecutorCreator{clientConfig: clientConfig}
	writer := block.NewPlannedBlockWriter[planData](plannerConfig, generator, executorCreator)
	return writer.Run()
}

type planData struct {
	expression string
	matcher    []*labels.Matcher
}

func (p planData) String() string {
	return p.expression
}

type planGenerator struct {
	matcherSets map[string][]*labels.Matcher
}

func (p *planGenerator) Generate(chunkStart int64, chunkEnd int64, stepDuration int64) []block.PlanEntry[planData] {
	var planEntries []block.PlanEntry[planData]
	for expression, matcher := range p.matcherSets {
		d := &planData{
			expression: expression,
			matcher:    matcher,
		}
		planEntries = append(planEntries, block.NewPlanEntry("migrate", chunkStart, chunkEnd, stepDuration, d))
	}
	return planEntries
}

type planExecutorCreator struct {
	clientConfig *remote.ClientConfig
}

func (p *planExecutorCreator) Create(name string, appender database.Appender) (block.PlanExecutor[planData], error) {
	client, err := remote.NewReadClient(name, p.clientConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create remote client")
	}
	return &planExecutor{
		client:   client,
		appender: appender,
	}, nil
}

type planExecutor struct {
	client   remote.ReadClient
	appender database.Appender
}

func (p *planExecutor) Execute(_ context.Context, logger block.PlanLogger[planData], plan block.PlanEntry[planData]) error {
	hints := &storage.SelectHints{
		Start: plan.Start(),
		End:   plan.End(),
		Step:  plan.Step(),
		Range: 0,
		Func:  "",
	}
	query, err := remote.ToQuery(hints.Start, hints.End, plan.Data().matcher, hints)
	if err != nil {
		return errors.Wrap(err, "failed to query remote")
	}

	var res *prompb.QueryResult
	err = backoff.Retry(func() error {
		r, e := p.client.Read(context.Background(), query)
		if e != nil {
			return e
		}
		res = r
		return nil
	}, backoff.WithMaxRetries(backoff.NewExponentialBackOff(), maxQueryRetryAttempts))
	if err != nil {
		return errors.Wrap(err, "failed reading remote data after %d attempts", maxQueryRetryAttempts)
	}
	sample := &promql.Sample{}
	for _, ts := range res.Timeseries {
		sample.Metric = common.LabelProtosToLabels(ts.Labels)
		for _, s := range ts.Samples {
			if value.IsStaleNaN(s.Value) || s.Timestamp < plan.Start() {
				continue
			}
			sample.T = s.Timestamp
			sample.V = s.Value
			if err = p.appender.Add(sample); err != nil {
				return errors.Wrap(err, "failed to add sample: %s", sample)
			}
		}
	}
	return nil
}

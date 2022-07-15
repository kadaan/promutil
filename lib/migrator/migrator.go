package migrator

import (
	"context"
	"fmt"
	"github.com/cenkalti/backoff"
	"github.com/kadaan/promutil/config"
	"github.com/kadaan/promutil/lib/block"
	"github.com/kadaan/promutil/lib/common"
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
	"time"
)

func Migrate(c *config.MigrateConfig) error {
	promUrl, err := url.Parse(fmt.Sprintf("%s://%s:%d/api/v1/read", c.Scheme, c.Host, c.Port))
	if err != nil {
		return errors.Wrap(err, "failed to parse promUrl")
	}

	clientConfig := &remote.ClientConfig{
		URL:              &promConfig.URL{URL: promUrl},
		Timeout:          model.Duration(2 * time.Minute),
		HTTPClientConfig: promConfig.HTTPClientConfig{},
		RetryOnRateLimit: true,
	}

	var matcherSets = make(map[string][]*labels.Matcher)
	for _, s := range c.MatcherSetExpressions {
		matchers, err := parser.ParseMetricSelector(s)
		if err != nil {
			return errors.Wrap(err, "failed to parse matcher")
		}
		matcherSets[s] = matchers
	}

	plannerConfig := block.NewPlannerConfig(c.OutputDirectory, c.Start, c.End, c.SampleInterval, int(c.Parallelism))
	generator := &planGenerator{matcherSets: matcherSets}
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
		logger.PrintExecutePlanError(plan, "could not create query", err)
		return err
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
		logger.PrintExecutePlanError(plan, "could not read data", err)
		return err
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
				return err
			}
		}
	}
	return nil
}

package generator

import (
	"context"
	"fmt"
	"github.com/PaesslerAG/gval"
	"github.com/kadaan/promutil/config"
	"github.com/kadaan/promutil/lib/block"
	"github.com/kadaan/promutil/lib/database"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"math"
)

var (
	expressionLanguage = gval.Full(
		gval.Function("Abs", math.Abs),
		gval.Function("Acos", math.Acos),
		gval.Function("Acosh", math.Acosh),
		gval.Function("Asin", math.Asin),
		gval.Function("Asinh", math.Asinh),
		gval.Function("Atan", math.Atan),
		gval.Function("Atan2", math.Atan2),
		gval.Function("Atanh", math.Atanh),
		gval.Function("Cbrt", math.Cbrt),
		gval.Function("Ceil", math.Ceil),
		gval.Function("Copysign", math.Copysign),
		gval.Function("Cos", math.Cos),
		gval.Function("Cosh", math.Cosh),
		gval.Function("Dim", math.Dim),
		gval.Function("Erf", math.Erf),
		gval.Function("Erfc", math.Erfc),
		gval.Function("Erfcinv", math.Erfcinv),
		gval.Function("Erfinv", math.Erfinv),
		gval.Function("Exp", math.Exp),
		gval.Function("Exp2", math.Exp2),
		gval.Function("Expm1", math.Expm1),
		gval.Function("FMA", math.FMA),
		gval.Function("Float32bits", math.Float32bits),
		gval.Function("Float32frombits", math.Float32frombits),
		gval.Function("Float64bits", math.Float64bits),
		gval.Function("Float64frombits", math.Float64frombits),
		gval.Function("Floor", math.Floor),
		gval.Function("Frexp", math.Frexp),
		gval.Function("Gamma", math.Gamma),
		gval.Function("Hypot", math.Hypot),
		gval.Function("Ilogb", math.Ilogb),
		gval.Function("Inf", math.Inf),
		gval.Function("IsInf", math.IsInf),
		gval.Function("IsNaN", math.IsNaN),
		gval.Function("J0", math.J0),
		gval.Function("J1", math.J1),
		gval.Function("Jn", math.Jn),
		gval.Function("Ldexp", math.Ldexp),
		gval.Function("Lgamma", math.Lgamma),
		gval.Function("Log", math.Log),
		gval.Function("Log10", math.Log10),
		gval.Function("Log1p", math.Log1p),
		gval.Function("Log2", math.Log2),
		gval.Function("Logb", math.Logb),
		gval.Function("Max", math.Max),
		gval.Function("Min", math.Min),
		gval.Function("Mod", math.Mod),
		gval.Function("Modf", math.Modf),
		gval.Function("NaN", math.NaN),
		gval.Function("Nextafter", math.Nextafter),
		gval.Function("Nextafter32", math.Nextafter32),
		gval.Function("Pow", math.Pow),
		gval.Function("Pow10", math.Pow10),
		gval.Function("Remainder", math.Remainder),
		gval.Function("Round", math.Round),
		gval.Function("RoundToEven", math.RoundToEven),
		gval.Function("Signbit", math.Signbit),
		gval.Function("Sin", math.Sin),
		gval.Function("Sincos", math.Sincos),
		gval.Function("Sinh", math.Sinh),
		gval.Function("Sqrt", math.Sqrt),
		gval.Function("Tan", math.Tan),
		gval.Function("Tanh", math.Tanh),
		gval.Function("Trunc", math.Trunc),
		gval.Function("Y0", math.Y0),
		gval.Function("Y1", math.Y1),
		gval.Function("Yn", math.Yn))
)

type metric struct {
	Name           string
	Expression     gval.Evaluable
	ExpressionText string
	Instances      []labels.Labels
	States         []*state
}

type state struct {
	Name      string
	Labels    map[string]string
	Index     float64
	Timestamp float64
	Last      float64
}

func Generate(c *config.GenerateConfig) error {
	metrics, err := createMetricSpecifications(c)
	if err != nil {
		return errors.Wrap(err, "failed to create metric specifications")
	}
	plannerConfig := block.NewPlannerConfig(c.OutputDirectory, c.Start, c.End, c.SampleInterval, int(c.Parallelism))
	generator := &planGenerator{metrics: metrics}
	executorCreator := &planExecutorCreator{}
	writer := block.NewPlannedBlockWriter[planData](plannerConfig, generator, executorCreator)
	return writer.Run()
}

func createMetricSpecifications(c *config.GenerateConfig) ([]*metric, error) {
	var metrics []*metric
	for _, timeSeriesConfig := range c.MetricConfig.TimeSeries {
		if expressionEngine, err := getExpressionEngine(timeSeriesConfig.Expression); err != nil {
			return nil, err
		} else {
			var lbls []labels.Labels
			var sts []*state
			if len(timeSeriesConfig.Instances) == 0 {
				for _, labelSet := range timeSeriesConfig.Labels {
					builder := labels.NewBuilder(labels.Labels{})
					for name, value := range labelSet {
						builder.Set(name, value)
					}
					st := state{
						Name:   timeSeriesConfig.Name,
						Labels: builder.Labels().Map(),
					}
					builder.Set(labels.MetricName, timeSeriesConfig.Name)
					lbls = append(lbls, builder.Labels())
					sts = append(sts, &st)
				}
			} else {
				for _, instance := range timeSeriesConfig.Instances {
					for _, labelSet := range timeSeriesConfig.Labels {
						builder := labels.NewBuilder(labels.Labels{})
						for name, value := range labelSet {
							builder.Set(name, value)
						}
						st := state{
							Name:   timeSeriesConfig.Name,
							Labels: builder.Labels().Map(),
						}
						builder.Set(labels.MetricName, timeSeriesConfig.Name)
						builder.Set(labels.InstanceName, instance)
						lbls = append(lbls, builder.Labels())
						sts = append(sts, &st)
					}
				}
			}
			metric := &metric{
				Name:           timeSeriesConfig.Name,
				Expression:     expressionEngine,
				ExpressionText: timeSeriesConfig.Expression,
				Instances:      lbls,
				States:         sts,
			}
			metrics = append(metrics, metric)
		}
	}
	return metrics, nil
}

func getExpressionEngine(expression string) (gval.Evaluable, error) {
	evaluable, err := expressionLanguage.NewEvaluable(expression)
	return evaluable, errors.Wrap(err, fmt.Sprintf("failed to parse expression: '%s'", expression))
}

type planData struct {
	metric *metric
}

func (p planData) String() string {
	return p.metric.Name
}

type planGenerator struct {
	metrics []*metric
}

func (p *planGenerator) Generate(chunkStart int64, chunkEnd int64, stepDuration int64) []block.PlanEntry[planData] {
	var planEntries []block.PlanEntry[planData]
	for _, metric := range p.metrics {
		d := &planData{
			metric: metric,
		}
		planEntries = append(planEntries, block.NewPlanEntry("generate", chunkStart, chunkEnd, stepDuration, d))
	}
	return planEntries
}

type planExecutorCreator struct {
}

func (p *planExecutorCreator) Create(_ string, appender database.Appender) (block.PlanExecutor[planData], error) {
	return &planExecutor{
		appender: appender,
	}, nil
}

type planExecutor struct {
	appender database.Appender
}

func (p *planExecutor) Execute(_ context.Context, _ block.PlanLogger[planData], plan block.PlanEntry[planData]) error {
	m := plan.Data().metric
	sample := &promql.Sample{}
	for i, instance := range m.Instances {
		s := m.States[i]
		for sampleTimestamp := plan.Start(); sampleTimestamp < plan.End(); sampleTimestamp += plan.Step() {
			s.Timestamp = float64(plan.Start())
			value, errE := m.Expression.EvalFloat64(context.Background(), s)
			if errE != nil {
				return errors.Wrap(errE, fmt.Sprintf("failed to evaluate expression %s", m.ExpressionText))
			}

			s.Last = value
			s.Index += 1
			if math.IsNaN(value) {
				continue
			}

			sample.T = sampleTimestamp
			sample.V = value
			sample.Metric = instance
			if err := p.appender.Add(sample); err != nil {
				return err
			}
		}
	}
	return nil
}

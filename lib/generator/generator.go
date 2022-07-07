package generator

import (
	"context"
	"fmt"
	"github.com/PaesslerAG/gval"
	"github.com/go-kit/kit/log"
	"github.com/kadaan/promutil/config"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	promRules "github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"math"
	"time"
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

type generator struct {
	config *generateConfig
}

type Generator interface {
	Generate() error
}

type generateConfig struct {
	OutputDir      string
	Logger         log.Logger
	Now            time.Time
	StartTime      time.Time
	EndTime        time.Time
	SampleInterval time.Duration
	BlockLength    time.Duration
	Metrics        []*metric
	RecordingRules config.RecordingRules
	Db             *tsdb.DB
	QueryFunc      promRules.QueryFunc
}

type metric struct {
	Name           string
	Expression     gval.Evaluable
	ExpressionText string
	Instances      []labels.Labels
	States         []*state
}

type state struct {
	Name      string
	Instance  string
	Labels    map[string]string
	Index     float64
	Timestamp float64
	Last      float64
}

func NewGenerator(c *config.GenerateConfig) (Generator, error) {
	var metrics []*metric
	for _, timeSeriesConfig := range c.MetricConfig.TimeSeries {
		if expressionEngine, err := getExpressionEngine(timeSeriesConfig.Expression); err != nil {
			return nil, nil
		} else {
			var lbls []labels.Labels
			var sts []*state
			for _, instance := range timeSeriesConfig.Instances {
				for _, labelSet := range timeSeriesConfig.Labels {
					builder := labels.NewBuilder(labels.Labels{})
					for name, value := range labelSet {
						builder.Set(name, value)
					}
					st := state{
						Name:     timeSeriesConfig.Name,
						Instance: instance,
						Labels:   builder.Labels().Map(),
					}
					builder.Set(labels.MetricName, timeSeriesConfig.Name)
					builder.Set(labels.InstanceName, instance)
					lbls = append(lbls, builder.Labels())
					sts = append(sts, &st)
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
	now := time.Now()
	queryEngine := promql.NewEngine(promql.EngineOpts{
		Logger:     log.NewNopLogger(),
		Reg:        prometheus.DefaultRegisterer,
		MaxSamples: 50_000_000,
		Timeout:    2 * time.Minute,
	})
	dbOptions := tsdb.DefaultOptions()
	dbOptions.NoLockfile = true
	dbOptions.AllowOverlappingBlocks = true

	registry := prometheus.NewRegistry()
	db, err := tsdb.Open(c.OutputDirectory, nil, registry, dbOptions, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open database")
	}
	db.EnableCompactions()
	queryFunc := promRules.EngineQueryFunc(queryEngine, db)
	cfg := &generateConfig{
		OutputDir:      c.OutputDirectory,
		Logger:         log.NewNopLogger(),
		Now:            now,
		StartTime:      c.Start,
		EndTime:        c.End,
		SampleInterval: c.SampleInterval,
		BlockLength:    c.BlockLength,
		Metrics:        metrics,
		RecordingRules: c.RuleConfig,
		Db:             db,
		QueryFunc:      queryFunc,
	}
	generator := &generator{config: cfg}
	return generator, nil
}

func (t generator) Generate() error {
	var samplesWritten int64
	for blockStart := t.config.StartTime; blockStart.Before(t.config.EndTime); blockStart = blockStart.Add(t.config.BlockLength) {
		blockEnd := blockStart.Add(t.config.BlockLength)
		fmt.Printf("Creating block from %s to %s...", blockStart.Format(time.RFC3339), blockEnd.Format(time.RFC3339))
		for sampleTimestamp := blockStart; sampleTimestamp.Before(blockEnd); sampleTimestamp = sampleTimestamp.Add(t.config.SampleInterval) {
			appender := t.config.Db.Appender(context.TODO())
			for _, metric := range t.config.Metrics {
				for i, instance := range metric.Instances {
					state := metric.States[i]
					state.Timestamp = float64(sampleTimestamp.UnixNano())
					value, err := metric.Expression.EvalFloat64(context.Background(), state)
					if err != nil {
						return errors.Wrap(err, fmt.Sprintf("failed to evaluate expression %s", metric.ExpressionText))
					}

					state.Last = value
					state.Index += 1
					if math.IsNaN(value) {
						continue
					}
					var ref storage.SeriesRef
					if _, err := appender.Append(ref, instance, sampleTimestamp.UnixNano()/int64(time.Millisecond/time.Nanosecond), value); err != nil {
						return errors.Wrap(err, fmt.Sprintf("failed to add sample for metric '%s'", metric.Name))
					}
					samplesWritten++
				}
			}
			err := appender.Commit()
			if err != nil {
				return errors.Wrap(err, "failed to commit metric samples")
			}
			appender = t.config.Db.Appender(context.TODO())
			for _, recordingRule := range t.config.RecordingRules {
				vector, err := t.config.QueryFunc(context.Background(), recordingRule.Query().String(), sampleTimestamp)
				if err != nil {
					return errors.Wrap(err, fmt.Sprintf("failed to run recording rule '%s'", recordingRule.Name))
				}
				for _, sample := range vector {
					lb := labels.NewBuilder(sample.Metric)
					lb.Set(labels.MetricName, recordingRule.Name())
					for _, l := range recordingRule.Labels() {
						lb.Set(l.Name, l.Value)
					}
					var ref storage.SeriesRef
					if _, err := appender.Append(ref, lb.Labels(), sample.T, sample.V); err != nil {
						return errors.Wrap(err, fmt.Sprintf("failed to add sample for recording rule '%s'", recordingRule.Name))
					}
					samplesWritten++
				}
			}
			err = appender.Commit()
			if err != nil {
				return errors.Wrap(err, "failed to commit samples recording rule samples")
			}
		}
		fmt.Print("  done\n")
	}
	fmt.Printf("Wrote %d samples\n", samplesWritten)
	return nil
}

func (t generator) Close() error {
	return t.config.Db.Close()
}

func getExpressionEngine(expression string) (gval.Evaluable, error) {
	evaluable, err := expressionLanguage.NewEvaluable(expression)
	return evaluable, errors.Wrap(err, fmt.Sprintf("failed to parse expression: '%s'", expression))
}

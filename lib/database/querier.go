package database

import (
	"context"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/kadaan/promutil/config"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/tsdb"
	"sort"
	"sync"
	"time"
)

type QueryManager interface {
	NewQuerier() Querier
	Close() error
}

type queryManager struct {
	mtx         *sync.RWMutex
	db          *tsdb.DB
	queryFunc   rules.QueryFunc
	queryEngine *promql.Engine
	stopped     bool
	resetFunc   func()
}

func newQueryManager(mtx *sync.RWMutex, db *tsdb.DB, resetFunc func()) (QueryManager, error) {
	registry := prometheus.NewRegistry()
	queryEngine := promql.NewEngine(promql.EngineOpts{
		Logger:     log.NewNopLogger(),
		Reg:        registry,
		MaxSamples: 500_000_000,
		Timeout:    10 * time.Minute,
	})
	return &queryManager{
		mtx:         mtx,
		db:          db,
		queryEngine: queryEngine,
		queryFunc:   rules.EngineQueryFunc(queryEngine, db),
		resetFunc:   resetFunc,
	}, nil
}

type RecordingRuleSampleIterator interface {
	Next() (bool, *promql.Sample)
}

type recordingRuleMatrixSampleIterator struct {
	recordingRule *config.RecordingRule
	currentSeries int
	series        []promql.Series
	seriesIndex   []int
	seriesLabels  []*labels.Labels
}

func newRecordingRuleSampleIterator(recordingRule *config.RecordingRule, matrix promql.Matrix) RecordingRuleSampleIterator {
	var seriesIndex []int
	var seriesLabels []*labels.Labels
	for _, series := range matrix {
		if len(series.Points) > 0 {
			seriesIndex = append(seriesIndex, 0)
			lb := labels.NewBuilder(series.Metric)
			lb.Set(labels.MetricName, recordingRule.Name())
			for _, l := range recordingRule.Labels() {
				lb.Set(l.Name, l.Value)
			}
			l := lb.Labels()
			sort.Sort(l)
			seriesLabels = append(seriesLabels, &l)
		}
	}
	return &recordingRuleMatrixSampleIterator{
		recordingRule: recordingRule,
		currentSeries: 0,
		series:        matrix,
		seriesIndex:   seriesIndex,
		seriesLabels:  seriesLabels,
	}
}

func (i *recordingRuleMatrixSampleIterator) Next() (bool, *promql.Sample) {
	if i.currentSeries >= len(i.series) {
		i.currentSeries = 0
		if i.currentSeries >= len(i.series) {
			return false, nil
		}
	}

	var result *promql.Sample = nil
	result = &promql.Sample{
		Point:  i.series[i.currentSeries].Points[i.seriesIndex[i.currentSeries]],
		Metric: *i.seriesLabels[i.currentSeries],
	}
	i.seriesIndex[i.currentSeries]++
	if i.seriesIndex[i.currentSeries] >= len(i.series[i.currentSeries].Points) {
		i.seriesIndex = append(i.seriesIndex[:i.currentSeries], i.seriesIndex[i.currentSeries+1:]...)
		i.seriesLabels = append(i.seriesLabels[:i.currentSeries], i.seriesLabels[i.currentSeries+1:]...)
		i.series = append(i.series[:i.currentSeries], i.series[i.currentSeries+1:]...)
	} else {
		i.currentSeries++
	}
	return true, result
}

func (m *queryManager) NewQuerier() Querier {
	return &querier{
		m: m,
	}
}

func (m *queryManager) Close() error {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	if m.stopped {
		return nil
	}
	m.stopped = true
	m.resetFunc()
	return nil
}

type Querier interface {
	QueryRangeRule(ctx context.Context, recordingRule *config.RecordingRule, start int64, end int64, step int64) (RecordingRuleSampleIterator, error)
}

type querier struct {
	m *queryManager
}

func (q *querier) QueryRangeRule(ctx context.Context, recordingRule *config.RecordingRule, start int64, end int64, step int64) (RecordingRuleSampleIterator, error) {
	if q.m.stopped {
		return nil, errors.New("cannot query with a closed querier")
	}
	q.m.mtx.RLock()
	defer q.m.mtx.RUnlock()
	qr, err := q.m.queryEngine.NewRangeQuery(q.m.db, nil, recordingRule.Query().String(), time.UnixMilli(start), time.UnixMilli(end), time.Duration(step)*time.Millisecond)

	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to run rule: expression=%s, start=%v, end=%v, step=%v", recordingRule.Query().String(), start, end, step))
	}
	res := qr.Exec(ctx)
	if res.Err != nil {
		return nil, errors.Wrap(res.Err, fmt.Sprintf("rule failed: expression=%s, start=%v, end=%v, step=%v", recordingRule.Query().String(), start, end, step))
	}
	switch v := res.Value.(type) {
	case promql.Matrix:
		return newRecordingRuleSampleIterator(recordingRule, v), nil
	default:
		return nil, errors.New("rule result is not a matrix")
	}
}

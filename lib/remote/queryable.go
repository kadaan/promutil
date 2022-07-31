package remote

import (
	"context"
	"fmt"
	"github.com/cenkalti/backoff"
	"github.com/cespare/xxhash/v2"
	"github.com/kadaan/promutil/lib/common"
	"github.com/kadaan/promutil/lib/errors"
	"github.com/kadaan/tracerr"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/rules"
	"io"
	"net/url"
	"sort"
	"sync"
	"time"
)

const (
	maxChunkDuration = 30 * time.Minute
)

func NewQueryable(address *url.URL, parallelism uint8) (Queryable, error) {
	client, err := api.NewClient(api.Config{
		Address: address.String(),
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create queryable provider")
	}
	ctx, cancel := context.WithCancel(context.Background())
	var cg sync.WaitGroup
	promApi := v1.NewAPI(client)
	inputChan := make(chan plan)
	for i := uint8(0); i < common.MaxUInt8(parallelism, uint8(1)); i++ {
		qr := querier{
			ctx:      ctx,
			cg:       &cg,
			promApi:  promApi,
			input:    inputChan,
			stopOnce: &sync.Once{},
		}
		cg.Add(1)
		go qr.run()
	}
	return &queryable{
		cg:        &cg,
		inputChan: inputChan,
		cancel:    cancel,
	}, nil
}

type queryable struct {
	cg        *sync.WaitGroup
	inputChan chan plan
	cancel    context.CancelFunc
}

func (q *queryable) Close() error {
	q.cancel()
	q.cg.Wait()
	return nil
}

func (q *queryable) QueryFuncProvider(minTimestamp time.Time, maxTimestamp time.Time, step time.Duration) (QueryFuncProvider, error) {
	return &queryFuncProvider{
		minTimestamp:     minTimestamp,
		maxTimestamp:     maxTimestamp,
		step:             step,
		inputChan:        q.inputChan,
		queryResultCache: map[string]map[time.Duration][]*queryCacheEntry{},
	}, nil
}

type RangeQueryFunc func(ctx context.Context, qs string, start time.Time, end time.Time, interval time.Duration) (promql.Matrix, error)

type Queryable interface {
	io.Closer
	QueryFuncProvider(minTimestamp time.Time, maxTimestamp time.Time, step time.Duration) (QueryFuncProvider, error)
}

type QueryFuncProvider interface {
	InstantQueryFunc(allowArbitraryQueries bool) rules.QueryFunc
	RangeQueryFunc() RangeQueryFunc
}

type queryCacheEntry struct {
	minTimestamp time.Time
	maxTimestamp time.Time
	query        string
	matrix       *promql.Matrix
	interval     time.Duration
	cached       bool
}

type queryFuncProvider struct {
	minTimestamp     time.Time
	maxTimestamp     time.Time
	step             time.Duration
	inputChan        chan plan
	queryResultCache map[string]map[time.Duration][]*queryCacheEntry
}

func (q *queryFuncProvider) seriesIndexHash(query string, start time.Time, end time.Time, interval time.Duration) uint64 {
	var buf []byte
	buf = append(buf, query...)
	buf = append(buf, fmt.Sprintf("%d", start.UnixNano())...)
	buf = append(buf, fmt.Sprintf("%d", end.UnixNano())...)
	buf = append(buf, fmt.Sprintf("%d", interval.Nanoseconds())...)
	return xxhash.Sum64(buf)
}

func (q *queryFuncProvider) InstantQueryFunc(allowArbitraryQueries bool) rules.QueryFunc {
	querySeriesIndex := make(map[uint64]*map[int]int)
	pQuerySeriesIndex := &querySeriesIndex
	return func(ctx context.Context, qs string, t time.Time) (promql.Vector, error) {
		result, err := q.query(ctx, qs, t, t, q.step, false, allowArbitraryQueries)
		if err != nil {
			return nil, err
		}
		var seriesIndex *map[int]int
		if result.cached {
			hash := q.seriesIndexHash(result.query, result.minTimestamp, result.maxTimestamp, result.interval)
			if _, ok := (*pQuerySeriesIndex)[hash]; !ok {
				m := make(map[int]int)
				for s := range *result.matrix {
					if _, ok2 := m[s]; !ok2 {
						m[s] = 0
					}
				}
				(*pQuerySeriesIndex)[hash] = &m
			}
			seriesIndex = (*pQuerySeriesIndex)[hash]
		}
		var vector promql.Vector
		ts := t.UnixMilli()
		for s, series := range *result.matrix {
			found := false
			seriesPos := 0
			if seriesIndex != nil {
				seriesPos = (*seriesIndex)[s]
			}
			seriesLen := len(series.Points)
			for ; seriesPos < seriesLen; seriesPos++ {
				point := series.Points[seriesPos]
				if ts < point.T {
					break
				}
				if point.T == ts {
					found = true
					vector = append(vector, promql.Sample{
						Point: promql.Point{
							T: timestamp.FromTime(t),
							V: point.V,
						},
						Metric: series.Metric,
					})
					break
				}
			}
			if seriesIndex != nil && found {
				(*seriesIndex)[s] = seriesPos + 1
			}
		}
		sort.Slice(vector, func(i, j int) bool {
			return vector[i].T-vector[j].T < 0
		})
		return vector, nil
	}
}

func (q *queryFuncProvider) RangeQueryFunc() RangeQueryFunc {
	return func(ctx context.Context, qs string, start time.Time, end time.Time, interval time.Duration) (promql.Matrix, error) {
		r, err := q.query(ctx, qs, start, end, interval, true, true)
		if err != nil {
			return nil, err
		}
		if r.minTimestamp.Equal(start) && r.maxTimestamp.Equal(end) {
			return *r.matrix, nil
		}
		minTs := start.UnixMilli()
		maxTs := end.UnixMilli()
		var matrix promql.Matrix
		for _, series := range *r.matrix {
			var points []promql.Point
			for _, point := range series.Points {
				if point.T >= minTs && point.T <= maxTs {
					points = append(points, point)
				}
			}
			if len(points) > 0 {
				matrix = append(matrix, promql.Series{
					Metric: series.Metric,
					Points: points,
				})
			}
		}
		return matrix, nil
	}
}

type plan struct {
	ctx        context.Context
	wg         *sync.WaitGroup
	queryRange v1.Range
	queryExpr  string
	output     chan<- result
	stopOnce   *sync.Once
	canceller  common.Canceller
}

type result struct {
	cancelled bool
	wg        *sync.WaitGroup
	err       tracerr.Error
	series    []*promql.Series
}

type querier struct {
	ctx      context.Context
	cg       *sync.WaitGroup
	promApi  v1.API
	input    <-chan plan
	stopOnce *sync.Once
}

func (q *querier) run() {
	for {
		select {
		case <-q.ctx.Done():
			q.stopOnce.Do(func() {
				q.cg.Done()
			})
			return
		case p, ok := <-q.input:
			if !ok {
				p.stopOnce.Do(func() {
					q.cg.Done()
				})
				return
			}
			if p.canceller.Cancelled() {
				p.output <- result{
					cancelled: true,
					wg:        p.wg,
				}
				continue
			}
			q.execute(p)
		}
	}
}

func (q *querier) execute(p plan) {
	var value *model.Value
	const attempts = 5
	err := backoff.Retry(func() error {
		r, _, e := q.promApi.QueryRange(p.ctx, p.queryExpr, p.queryRange)
		if e != nil {
			return e
		}
		value = &r
		return nil
	}, backoff.WithMaxRetries(backoff.NewExponentialBackOff(), attempts))
	if err != nil {
		p.output <- result{
			wg:  p.wg,
			err: errors.Wrap(err, "failed to query '%s' from %d to %d after %d attempts", p.queryExpr, p.queryRange.Start, p.queryRange.End, attempts),
		}
		return
	}

	switch v := (*value).(type) {
	case model.Matrix:
		var series []*promql.Series
		for _, ss := range v {
			var points []promql.Point
			for _, s := range ss.Values {
				points = append(points, promql.Point{
					T: common.TimeMilliseconds(s.Timestamp),
					V: float64(s.Value),
				})
			}
			var metric labels.Labels
			for k, v := range ss.Metric {
				metric = append(metric, labels.Label{
					Name:  string(k),
					Value: string(v),
				})
			}
			sort.Sort(metric)
			series = append(series, &promql.Series{
				Metric: metric,
				Points: points,
			})
		}
		p.output <- result{
			wg:     p.wg,
			series: series,
		}
	default:
		p.output <- result{
			wg:  p.wg,
			err: errors.New("query range result is not a matrix"),
		}
	}
}

func (q *queryFuncProvider) query(ctx context.Context, query string, start time.Time, end time.Time, interval time.Duration, addToCache bool, allowArbitraryQueries bool) (*queryCacheEntry, error) {
	if queryCache, ok := q.queryResultCache[query]; ok {
		if intervalCache, ok2 := queryCache[interval]; ok2 {
			for _, entry := range intervalCache {
				if start.Before(entry.minTimestamp) ||
					start.After(entry.maxTimestamp) ||
					end.Before(entry.minTimestamp) ||
					end.After(entry.maxTimestamp) {
					// Skip because the request query is not a subset of this query
				} else {
					return entry, nil
				}
			}
		}
	}

	if !allowArbitraryQueries {
		var matrix promql.Matrix
		entry := &queryCacheEntry{
			minTimestamp: start,
			maxTimestamp: end,
			interval:     interval,
			query:        query,
			matrix:       &matrix,
			cached:       false,
		}
		return entry, nil
	}

	pCtx, cancel := context.WithCancel(ctx)
	canceller := common.NewCanceller()
	outputChan := make(chan result)
	defer close(outputChan)

	var wg sync.WaitGroup
	var err tracerr.Error
	metricSeriesMap := make(map[uint64]*promql.Series)
	go func(canceller common.Canceller) {
		for {
			select {
			case s, ok := <-outputChan:
				if !ok {
					return
				}
				if s.err != nil {
					err = s.err
					canceller.Cancel()
				} else if !s.cancelled {
					for _, ss := range s.series {
						metricHash := ss.Metric.Hash()
						if series, exists := metricSeriesMap[metricHash]; !exists {
							metricSeriesMap[metricHash] = ss
						} else {
							series.Points = append(series.Points, ss.Points...)
						}
					}
				}
				s.wg.Done()
			}
		}
	}(canceller)

	chunkStart := start
	for ; !chunkStart.After(end); chunkStart = chunkStart.Add(maxChunkDuration) {
		chunkEnd := common.MinTime(chunkStart.Add(maxChunkDuration).Add(-1*time.Nanosecond), end)
		wg.Add(1)
		q.inputChan <- plan{
			canceller: canceller,
			ctx:       pCtx,
			wg:        &wg,
			queryRange: v1.Range{
				Start: chunkStart,
				End:   chunkEnd,
				Step:  interval,
			},
			queryExpr: query,
			output:    outputChan,
			stopOnce:  &sync.Once{},
		}
	}

	go func() {
		select {
		case <-ctx.Done():
			canceller.Cancel()
			cancel()
		case <-canceller.C():
			cancel()
		}
	}()

	wg.Wait()
	cancel()

	if err != nil {
		return nil, err
	}

	var matrix promql.Matrix
	for _, s := range metricSeriesMap {
		sort.Slice(s.Points, func(i, j int) bool {
			return s.Points[i].T-s.Points[j].T < 0
		})
		matrix = append(matrix, *s)
	}

	entry := &queryCacheEntry{
		minTimestamp: start,
		maxTimestamp: end,
		interval:     interval,
		query:        query,
		matrix:       &matrix,
		cached:       false,
	}

	if addToCache {
		entry.cached = true
		if _, exists := q.queryResultCache[query]; !exists {
			q.queryResultCache[query] = make(map[time.Duration][]*queryCacheEntry)
		}
		if _, exists := q.queryResultCache[query][interval]; !exists {
			q.queryResultCache[query][interval] = make([]*queryCacheEntry, 0)
		}
		q.queryResultCache[query][interval] = append(q.queryResultCache[query][interval], entry)
	}

	return entry, nil
}

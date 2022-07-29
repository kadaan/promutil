package remote

import (
	"context"
	"fmt"
	"github.com/cenkalti/backoff"
	"github.com/cespare/xxhash/v2"
	"github.com/kadaan/promutil/lib/common"
	"github.com/kadaan/promutil/lib/errors"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/rules"
	"math"
	"net/url"
	"sort"
	"time"
	//"time"
)

func NewQueryable(address *url.URL) (Queryable, error) {
	client, err := api.NewClient(api.Config{
		Address: address.String(),
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create queryable provider")
	}
	return &queryable{
		client: client,
	}, nil
}

type RangeQueryFunc func(ctx context.Context, qs string, start time.Time, end time.Time, interval time.Duration) (promql.Matrix, error)

type Queryable interface {
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
	stats        []seriesStats
	cached       bool
}

type queryFuncProvider struct {
	promApi          v1.API
	minTimestamp     time.Time
	maxTimestamp     time.Time
	step             time.Duration
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
		result, err := q.query(ctx, qs, start, end, interval, true, true)
		if err != nil {
			return nil, err
		}
		if result.minTimestamp.Equal(start) && result.maxTimestamp.Equal(end) {
			return *result.matrix, nil
		}
		minTs := start.UnixMilli()
		maxTs := end.UnixMilli()
		var matrix promql.Matrix
		for _, series := range *result.matrix {
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

type seriesStats struct {
	MinTimestamp int64
	MaxTimestamp int64
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

	rng := v1.Range{
		Start: start,
		End:   end,
		Step:  interval,
	}

	var value *model.Value
	const attempts = 5
	err := backoff.Retry(func() error {
		r, _, e := q.promApi.QueryRange(ctx, query, rng)
		if e != nil {
			return e
		}
		value = &r
		return nil
	}, backoff.WithMaxRetries(backoff.NewExponentialBackOff(), attempts))
	if err != nil {
		return nil, errors.Wrap(err, "failed to query '%s' from %d to %d after %d attempts", query, rng.Start, rng.End, attempts)
	}
	var stats []seriesStats
	var matrix promql.Matrix
	switch v := (*value).(type) {
	case model.Matrix:
		for _, ss := range v {
			stat := seriesStats{
				MinTimestamp: math.MaxInt64,
				MaxTimestamp: math.MinInt64,
			}
			var points []promql.Point
			for _, s := range ss.Values {
				ts := common.TimeMilliseconds(s.Timestamp)
				if ts < stat.MinTimestamp {
					stat.MinTimestamp = ts
				}
				if ts > stat.MaxTimestamp {
					stat.MaxTimestamp = ts
				}
				points = append(points, promql.Point{
					T: ts,
					V: float64(s.Value),
				})
			}
			for _, s := range matrix {
				sort.Slice(s.Points, func(i, j int) bool {
					return s.Points[i].T-s.Points[j].T < 0
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
			sort.Slice(points, func(i, j int) bool {
				return points[i].T-points[j].T < 0
			})
			matrix = append(matrix, promql.Series{
				Metric: metric,
				Points: points,
			})
			stats = append(stats, stat)
		}
	default:
		return nil, errors.New("query range result is not a matrix")
	}

	entry := &queryCacheEntry{
		minTimestamp: rng.Start,
		maxTimestamp: rng.End,
		interval:     interval,
		query:        query,
		matrix:       &matrix,
		stats:        stats,
		cached:       false,
	}

	if addToCache {
		entry.cached = true
		if _, exists := q.queryResultCache[query]; !exists {
			q.queryResultCache[query] = make(map[time.Duration][]*queryCacheEntry)
		}
		if _, exists := q.queryResultCache[query][rng.Step]; !exists {
			q.queryResultCache[query][rng.Step] = make([]*queryCacheEntry, 0)
		}
		q.queryResultCache[query][rng.Step] = append(q.queryResultCache[query][rng.Step], entry)
	}

	return entry, nil
}

type queryable struct {
	client api.Client
}

func (q *queryable) QueryFuncProvider(minTimestamp time.Time, maxTimestamp time.Time, step time.Duration) (QueryFuncProvider, error) {
	promApi := v1.NewAPI(q.client)
	return &queryFuncProvider{
		promApi:          promApi,
		minTimestamp:     minTimestamp,
		maxTimestamp:     maxTimestamp,
		step:             step,
		queryResultCache: map[string]map[time.Duration][]*queryCacheEntry{},
	}, nil
}

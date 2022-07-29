package common

import (
	"fmt"
	"github.com/kadaan/promutil/lib/downsample"
	"github.com/kadaan/promutil/lib/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql"
	"math"
	"sort"
	"strconv"
	"time"
)

var (
	minTime = time.Unix(math.MinInt64/1000+62135596801, 0).UTC()
	maxTime = time.Unix(math.MaxInt64/1000-62135596801, 999999999).UTC()

	minTimeFormatted = minTime.Format(time.RFC3339Nano)
	maxTimeFormatted = maxTime.Format(time.RFC3339Nano)
)

func FormatDateRange(start int64, end int64) string {
	return fmt.Sprintf("%s to %s", FormatDate(start), FormatDate(end))
}

func FormatDate(value int64) string {
	return time.UnixMilli(value).Format("2006-01-02T15:04:05")
}

func LabelProtosToLabels(labelPairs []prompb.Label) labels.Labels {
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

func DownsampleMatrix(matrix promql.Matrix, maxSamples int, avg bool) promql.Matrix {
	var newMatrix promql.Matrix
	downsampler := downsample.NewLttbDownsampler()
	for _, series := range matrix {
		series.Points = downsampler.Downsample(series.Points, maxSamples)
		newMatrix = append(newMatrix, series)
	}
	return newMatrix
}

func ParseTime(s string) (time.Time, error) {
	if t, err := strconv.ParseFloat(s, 64); err == nil {
		s, ns := math.Modf(t)
		ns = math.Round(ns*1000) / 1000
		return time.Unix(int64(s), int64(ns*float64(time.Second))).UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}

	// Stdlib's time parser can only handle 4 digit years. As a workaround until
	// that is fixed we want to at least support our own boundary times.
	// Context: https://github.com/prometheus/client_golang/issues/614
	// Upstream issue: https://github.com/golang/go/issues/20555
	switch s {
	case minTimeFormatted:
		return minTime, nil
	case maxTimeFormatted:
		return maxTime, nil
	}
	return time.Time{}, errors.New("cannot parse %q to a valid timestamp", s)
}

func ParseDuration(s string) (time.Duration, error) {
	if d, err := strconv.ParseFloat(s, 64); err == nil {
		ts := d * float64(time.Second)
		if ts > float64(math.MaxInt64) || ts < float64(math.MinInt64) {
			return 0, errors.Errorf("cannot parse %q to a valid duration. It overflows int64", s)
		}
		return time.Duration(ts), nil
	}
	if d, err := model.ParseDuration(s); err == nil {
		return time.Duration(d), nil
	}
	return 0, errors.Errorf("cannot parse %q to a valid duration", s)
}

func TimeMilliseconds(t model.Time) int64 {
	return t.UnixNano() / int64(time.Millisecond/time.Nanosecond)
}

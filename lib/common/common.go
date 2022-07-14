package common

import (
	"fmt"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"sort"
	"time"
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

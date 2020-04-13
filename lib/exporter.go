package lib

import (
	"context"
	"fmt"
	"github.com/json-iterator/go"
	"github.com/kadaan/promutil/config"
	"github.com/pkg/errors"
	prom_config "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/pkg/value"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"net/url"
	"os"
	"sort"
	"time"
)

type exporter struct {
	config *exportConfig
}

type Exporter interface {
	Export() error
}

type exportConfig struct {
	Host           string
	Port           uint16
	StartTime      time.Time
	EndTime        time.Time
	SampleInterval time.Duration
	MatcherSets    map[string][]*labels.Matcher
}

type kv struct {
	Key   string
	Value int
}

func NewExporter(c *config.ExportConfig) (Exporter, error) {
	now := time.Now()

	var matcherSets = make(map[string][]*labels.Matcher)
	for _, s := range c.MatcherSetExpressions {
		matchers, err := parser.ParseMetricSelector(s)
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse matcher")
		}
		matcherSets[s] = matchers
	}

	config := &exportConfig{
		Host:           c.Host,
		Port:           c.Port,
		StartTime:      now.Add(-c.Duration),
		EndTime:        now,
		SampleInterval: c.SampleInterval,
		MatcherSets:    matcherSets,
	}
	return exporter{config }, nil
}

func (e exporter) Export() error {
	url, err := url.Parse(fmt.Sprintf("http://%s:%d/api/v1/read", e.config.Host, e.config.Port))
	if err != nil {
		return errors.Wrap(err, "failed to parse url")
	}

	clientConfig := &remote.ClientConfig{
		URL:              &prom_config.URL{URL: url},
		Timeout:          model.Duration(2 * time.Minute),
		HTTPClientConfig: prom_config.HTTPClientConfig{},
	}

	client, err := remote.NewClient("remote", clientConfig)
	if err != nil {
		return errors.Wrap(err, "failed to create remote client")
	}

	stream := jsoniter.NewStream(jsoniter.ConfigDefault, os.Stdout, 4096)

	blockLength := 2 * time.Hour
	blockStart := e.config.StartTime
	for blockStart.Before(e.config.EndTime) {
		blockEnd := blockStart.Add(blockLength)
		if blockEnd.After(e.config.EndTime) {
			blockEnd = e.config.EndTime
		}
		if err := e.export(client, stream, blockStart, blockEnd); err != nil {
			return err
		}
		blockStart = blockStart.Add(blockLength)
	}
	return errors.Wrap(stream.Flush(), "failed to serialize")
}

type block struct {
	Start      	int64  				`json:"start"`
	End 		int64  				`json:"end"`
	Step	   	int64  				`json:"step"`
	Strings     []string            `json:"strings"`
	Labels     	[][]int				`json:"labels"`
	Timestamps 	[][]interface{} 	`json:"timestamps"`
	Values    	[][]interface{} 	`json:"values"`
}

func (e exporter) export(client *remote.Client, stream *jsoniter.Stream, startTime time.Time, endTime time.Time) error {
	hints := &storage.SelectHints{
		Start: timestamp.FromTime(startTime),
		End:   timestamp.FromTime(endTime),
		Step:  int64(e.config.SampleInterval / time.Millisecond),
	}

	block := block{
		Start:   	hints.Start,
		End:   		hints.End,
		Step:		hints.Step,
		Strings: 	[]string{},
		Labels: 	[][]int{},
		Timestamps: [][]interface{}{},
		Values:     [][]interface{}{},
	}

	stringCount := 0
	strings := map[string]int{}
	for e, m := range e.config.MatcherSets {
		fmt.Fprintf(os.Stderr, "Exporting '%s' from %s to %s...\n", e, startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))
		query, err := remote.ToQuery(hints.Start, hints.End, m, hints)
		if err != nil {
			return errors.Wrap(err, "failed to create query")
		}
		res, err := client.Read(context.Background(), query)
		if err != nil {
			return errors.Wrap(err, "failed to read data")
		}

		for _, ts := range res.Timeseries {
			var labels []int
			for _, l := range ts.Labels {
				var nameId int
				var valueId int
				if id, ok := strings[l.Name]; !ok {
					nameId = stringCount
					strings[l.Name] = nameId
					stringCount++
				} else {
					nameId = id
				}
				if id, ok := strings[l.Value]; !ok {
					valueId = stringCount
					strings[l.Value] = valueId
					stringCount++
				} else {
					valueId = id
				}
				labels = append(labels, nameId, valueId)
			}
			block.Labels = append(block.Labels, labels)
			timestamps, values := encodeSamples(ts.Samples)
			block.Timestamps = append(block.Timestamps, timestamps)
			block.Values = append(block.Values, values)
		}

		var ss []kv
		for k, v := range strings {
			ss = append(ss, kv{k, v})
		}

		sort.Slice(ss, func(i, j int) bool {
			return ss[i].Value < ss[j].Value
		})

		for _, kv := range ss {
			block.Strings = append(block.Strings, kv.Key)
		}
		stream.WriteVal(block)
		stream.WriteRaw("\n")
	}
	return errors.Wrap(stream.Flush(), "failed to serialize")
}

func encodeSamples(samples []prompb.Sample) ([]interface{}, []interface{}) {
	var timestampState = 0
	var valueState = 0
	var lastTimestamp int64
	var lastValue float64
	var lastTimestampDelta int64
	var lastValueDelta float64
	var priorTimestamp int64
	var priorValue float64
	var duplicateTimestampCount int64
	var duplicateValueCount int64
	var nextTimestamp int64
	var nextValue float64

	timestamps := make([]interface{}, 0)
	values := make([]interface{}, 0)
	for _, s := range samples {
		if value.IsStaleNaN(s.Value) {
			continue
		}

		if timestampState == 0 {
			priorTimestamp = s.Timestamp
			lastTimestamp = s.Timestamp
			duplicateTimestampCount = 0
			timestampState = 1
		} else {
			delta := s.Timestamp - lastTimestamp
			if timestampState == 1 {
				nextTimestamp = delta
				timestampState = 2
			} else {
				nextTimestamp = delta - lastTimestampDelta
			}
			lastTimestampDelta = delta
			lastTimestamp = s.Timestamp
			if nextTimestamp != priorTimestamp {
				if duplicateTimestampCount > 0 {
					timestamps = append(timestamps, fmt.Sprintf("%dX%d", duplicateTimestampCount, priorTimestamp))
					duplicateTimestampCount = 0
				} else {
					timestamps = append(timestamps, priorTimestamp)
				}
				priorTimestamp = nextTimestamp
			} else {
				duplicateTimestampCount++
			}
		}

		if valueState == 0 {
			priorValue = s.Value
			lastValue = s.Value
			duplicateValueCount = 0
			valueState = 1
		} else {
			delta := s.Value - lastValue
			if valueState == 1 {
				nextValue = delta
				valueState = 2
			} else {
				nextValue = delta - lastValueDelta
			}
			lastValueDelta = delta
			lastValue = s.Value
			if nextValue != priorValue {
				if duplicateValueCount > 0 {
					values = append(values, fmt.Sprintf("%dX%f", duplicateValueCount, priorValue))
					duplicateValueCount = 0
				} else {
					values = append(values, priorValue)
				}
				priorValue = nextValue
			} else {
				duplicateValueCount++
			}
		}
	}

	if duplicateTimestampCount > 0 {
		timestamps = append(timestamps, fmt.Sprintf("%dX%d", duplicateTimestampCount, priorTimestamp))
	} else {
		timestamps = append(timestamps, priorTimestamp)
	}

	if duplicateValueCount > 0 {
		values = append(values, fmt.Sprintf("%dX%f", duplicateValueCount, priorValue))
	} else {
		values = append(values, priorValue)
	}

	return timestamps, values
}
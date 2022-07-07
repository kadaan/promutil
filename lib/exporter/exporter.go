package exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/kadaan/promutil/config"
	"github.com/kadaan/promutil/lib/block"
	"github.com/klauspost/compress/zstd"
	"github.com/pkg/errors"
	promConfig "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"net/url"
	"os"
	"time"
)

type exporter struct {
	config *exportConfig
}

type Exporter interface {
	Export() error
}

type exportConfig struct {
	OutputFile     string
	Scheme         string
	Host           string
	Port           uint16
	StartTime      time.Time
	EndTime        time.Time
	SampleInterval time.Duration
	MatcherSets    map[string][]*labels.Matcher
}

func Export(c *config.ExportConfig) error {
	var matcherSets = make(map[string][]*labels.Matcher)
	for _, s := range c.MatcherSetExpressions {
		matchers, err := parser.ParseMetricSelector(s)
		if err != nil {
			return errors.Wrap(err, "failed to parse matcher")
		}
		matcherSets[s] = matchers
	}

	cfg := &exportConfig{
		OutputFile:     c.OutputFile,
		Scheme:         string(c.Scheme),
		Host:           c.Host,
		Port:           c.Port,
		StartTime:      c.Start,
		EndTime:        c.End,
		SampleInterval: c.SampleInterval,
		MatcherSets:    matcherSets,
	}
	exporter := exporter{cfg}
	err := exporter.Export()
	if err != nil {
		return errors.Wrap(err, "failed to export")
	}
	return nil
}

func (e exporter) Export() error {
	promUrl, err := url.Parse(fmt.Sprintf("%s://%s:%d/api/v1/read", e.config.Scheme, e.config.Host, e.config.Port))
	if err != nil {
		return errors.Wrap(err, "failed to parse url")
	}

	clientConfig := &remote.ClientConfig{
		URL:              &promConfig.URL{URL: promUrl},
		Timeout:          model.Duration(2 * time.Minute),
		HTTPClientConfig: promConfig.HTTPClientConfig{},
		RetryOnRateLimit: true,
	}

	client, err := remote.NewReadClient("remote", clientConfig)
	if err != nil {
		return errors.Wrap(err, "failed to create remote client")
	}

	f, err := os.Create(e.config.OutputFile)
	if err != nil {
		return errors.Wrap(err, "failed to create output file")
	}
	defer func(f *os.File) {
		_ = f.Close()
	}(f)

	writer, err := zstd.NewWriter(f, zstd.WithEncoderCRC(true), zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		return errors.Wrap(err, "failed to create writer")
	}
	defer func(writer *zstd.Encoder) {
		_ = writer.Close()
	}(writer)

	stream := json.NewEncoder(writer)

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
	return nil
}

func DurationMilliseconds(d time.Duration) int64 {
	return int64(d / (time.Millisecond / time.Nanosecond))
}

func (e exporter) export(client remote.ReadClient, stream *json.Encoder, startTime time.Time, endTime time.Time) error {
	start := timestamp.FromTime(startTime)
	end := timestamp.FromTime(endTime)

	hints := &storage.SelectHints{
		Start: start,
		End:   end,
		Step:  DurationMilliseconds(e.config.SampleInterval),
		Range: 0,
		Func:  "",
	}

	blockBuilder := block.NewBlockBuilder(hints.Start, hints.End, hints.Step)
	for e, m := range e.config.MatcherSets {
		_, _ = fmt.Fprintf(os.Stderr, "Exporting '%s' from %s to %s...\n", e, startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))
		query, err := remote.ToQuery(hints.Start, hints.End, m, hints)
		if err != nil {
			return errors.Wrap(err, "failed to create query")
		}
		res, err := client.Read(context.Background(), query)
		if err != nil {
			return errors.Wrap(err, "failed to read data")
		}
		err = blockBuilder.Add(res.Timeseries)
		if err != nil {
			return err
		}
	}
	blk := blockBuilder.GetBlock()

	return stream.Encode(blk)
}

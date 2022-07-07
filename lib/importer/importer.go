package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/kadaan/promutil/config"
	"github.com/kadaan/promutil/lib/block"
	"github.com/klauspost/compress/zstd"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"os"
)

const (
	CLEAR   = "\033[K"
	SAVE    = "\033[s"
	RESTORE = "\033[u"
	RESET   = "\033[0;0m"
)

type importer struct {
	config *importConfig
}

type Importer interface {
	Import() error
}

type importConfig struct {
	DataFiles []string
	OutputDir string
	Logger    log.Logger
	Db        *tsdb.DB
}

func Import(c *config.ImportConfig) error {
	dbOptions := tsdb.DefaultOptions()
	dbOptions.NoLockfile = true
	dbOptions.AllowOverlappingBlocks = true

	registry := prometheus.NewRegistry()
	db, err := tsdb.Open(c.OutputDirectory, nil, registry, dbOptions, nil)
	if err != nil {
		return errors.Wrap(err, "failed to open database")
	}
	defer func(db *tsdb.DB) {
		_ = db.Close()
	}(db)
	db.EnableCompactions()

	cfg := &importConfig{
		DataFiles: c.DataFiles,
		OutputDir: c.OutputDirectory,
		Logger:    log.NewNopLogger(),
		Db:        db,
	}

	importer := importer{cfg}
	err = importer.Import()
	if err != nil {
		return errors.Wrap(err, "failed to import")
	}
	return nil
}

func (i importer) Import() error {
	decoderCreated := false
	var decoder *zstd.Decoder
	appender := i.config.Db.Appender(context.TODO())
	for _, dataFile := range i.config.DataFiles {
		_, _ = fmt.Fprintf(os.Stderr, "Importing '%s'%s", dataFile, CLEAR)
		err := i.importDataFile(decoder, dataFile, appender)
		if !decoderCreated && decoder != nil {
			decoderCreated = true
			defer decoder.Close()
		}
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("failed to import data file '%s'", dataFile))
		}
	}
	_, _ = fmt.Fprintf(os.Stderr, "Committing data\n")
	err := appender.Commit()
	if err != nil {
		return errors.Wrap(err, "failed to commit metric samples")
	}
	return nil
}

func (i importer) importDataFile(decoder *zstd.Decoder, dataFile string, appender storage.Appender) error {
	f, err := os.Open(dataFile)
	if err != nil {
		return errors.Wrap(err, "failed to open data file")
	}
	defer func(f *os.File) {
		_ = f.Close()
	}(f)
	if decoder == nil {
		d, err := zstd.NewReader(f)
		if err != nil {
			return err
		}
		decoder = d
	} else {
		err := decoder.Reset(f)
		if err != nil {
			return err
		}
	}

	iterator := json.NewDecoder(decoder)

	blockCount := 0
	sampleCount := 0
	var blk = block.NewEmptyBlock()
	for iterator.More() {
		blockCount++
		err := iterator.Decode(blk)
		if err != nil {
			return errors.Wrap(err, "failed to read value")

		}
		_, _ = fmt.Fprintf(os.Stderr, "\r%sImporting '%s': Loading Block %d%s", RESET, dataFile, blockCount, CLEAR)
		sampleIterator := blk.NewIterator()
		sampleNum := 0
		for {
			more, err, next := sampleIterator.Next()
			if err != nil {
				return err
			}
			if sampleNum == 0 {
				_, _ = fmt.Fprintf(os.Stderr, ", Sample %s%d%s", SAVE, sampleNum, CLEAR)
			} else if sampleNum%100 == 0 {
				_, _ = fmt.Fprintf(os.Stderr, "%s%s%d%s", RESTORE, RESET, sampleNum, CLEAR)
			}
			var ref storage.SeriesRef
			_, err = appender.Append(ref, next.Metric, next.T, next.V)
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, " - FAILED\n")
				return errors.Wrap(err, fmt.Sprintf("failed to add sample for metric '%s' from block %d "+
					"on line %d", next.Metric, blockCount, blockCount-1))
			}
			sampleNum++
			if !more {
				break
			}
		}
		sampleCount += sampleNum
	}
	_, _ = fmt.Fprintf(os.Stderr, "\rImported %d samples in %d blocks from %s%s\n", sampleCount,
		blockCount, dataFile, CLEAR)
	return nil
}

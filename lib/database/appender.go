package database

import (
	"context"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	errors2 "github.com/prometheus/prometheus/tsdb/errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

const (
	maxBlockFlushDuration = int64(72 * time.Hour / time.Millisecond)
)

type AppendManager interface {
	NewAppender() (Appender, error)
	Close() error
}

type appendManager struct {
	context       context.Context
	mtx           *sync.RWMutex
	db            *tsdb.DB
	gen           uint64
	dir           string
	blockDuration int64
	appenders     []Appender
	stopped       bool
	resetFunc     func()
}

func newAppendManager(ctx context.Context, mtx *sync.RWMutex, dir string, blockDuration int64, resetFunc func()) (AppendManager, error) {
	return &appendManager{
		context:       ctx,
		mtx:           mtx,
		dir:           dir,
		blockDuration: blockDuration,
		resetFunc:     resetFunc,
	}, nil
}

func (a *appendManager) NewAppender() (Appender, error) {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	appenderDir := filepath.Join(a.dir, fmt.Sprintf("%d", a.gen))
	a.gen++
	blockFlushDuration := a.blockDuration
	if blockFlushDuration > maxBlockFlushDuration {
		blockFlushDuration = maxBlockFlushDuration
	}
	appender := &safeAppender{
		context:            a.context,
		mtx:                a.mtx,
		maxSamplesInMemory: 15000,
		destDir:            a.dir,
		dir:                appenderDir,
		blockStart:         -1,
		blockDuration:      a.blockDuration,
		blockFlushDuration: blockFlushDuration,
	}
	err := appender.newBlockWriter(appenderDir, a.blockDuration)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create block writer")
	}
	a.appenders = append(a.appenders, appender)
	return appender, nil
}

func (a *appendManager) Close() error {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	if a.stopped {
		return nil
	}
	a.stopped = true
	var errs []error
	for _, appender := range a.appenders {
		err := appender.close()
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors2.NewMulti(errs...).Err()
	}
	a.resetFunc()
	return nil
}

type Appender interface {
	Add(sample *promql.Sample) error
	close() error
}

type safeAppender struct {
	context            context.Context
	mtx                *sync.RWMutex
	maxSamplesInMemory uint64
	currentSampleCount uint64
	blockWriter        *tsdb.BlockWriter
	destDir            string
	dir                string
	blockStart         int64
	blockDuration      int64
	blockFlushDuration int64
	appender           *storage.Appender
	stopped            bool
}

func (a *safeAppender) Add(sample *promql.Sample) error {
	if a.stopped {
		return errors.New("cannot append to a closed appender")
	}
	var err error
	a.mtx.RLock()
	if a.blockStart < 0 {
		a.blockStart = sample.T
	} else if sample.T > a.blockStart+a.blockFlushDuration {
		a.mtx.RUnlock()
		a.mtx.Lock()
		defer a.mtx.Unlock()
		errF := a.flush()
		if errF != nil {
			return errF
		}
		a.blockStart = a.blockStart + a.blockFlushDuration
		if _, err = (*a.appender).Append(0, sample.Metric, sample.T, sample.V); err != nil {
			defer a.mtx.RUnlock()
			return err
		}
		return nil
	}

	if _, err = (*a.appender).Append(0, sample.Metric, sample.T, sample.V); err != nil {
		defer a.mtx.RUnlock()
		return err
	}
	if atomic.AddUint64(&a.currentSampleCount, 1) >= a.maxSamplesInMemory {
		a.mtx.RUnlock()
		a.mtx.Lock()
		defer a.mtx.Unlock()
		if atomic.LoadUint64(&a.currentSampleCount) >= a.maxSamplesInMemory {
			if err = (*a.appender).Commit(); err != nil {
				return err
			}
			appender := a.blockWriter.Appender(a.context)
			a.appender = &appender
			atomic.StoreUint64(&a.currentSampleCount, 0)
		}
	} else {
		a.mtx.RUnlock()
	}
	return nil
}

func (a *safeAppender) flush() error {
	if a.stopped {
		return errors.New("cannot flush a closed appender")
	}
	if atomic.LoadUint64(&a.currentSampleCount) > 0 {
		if err := (*a.appender).Commit(); err != nil {
			return err
		}
		atomic.StoreUint64(&a.currentSampleCount, 0)
	}
	if _, err := a.blockWriter.Flush(a.context); err != nil {
		return err
	}
	err := a.newBlockWriter(a.dir, a.blockDuration)
	if err != nil {
		return err
	}
	appender := a.blockWriter.Appender(a.context)
	a.appender = &appender
	return nil
}

func (a *safeAppender) close() error {
	if a.stopped {
		return nil
	}
	a.stopped = true
	if atomic.LoadUint64(&a.currentSampleCount) > 0 {
		if err := (*a.appender).Commit(); err != nil {
			return err
		}
		atomic.StoreUint64(&a.currentSampleCount, 0)
	}
	if _, err := a.blockWriter.Flush(a.context); err != nil {
		return err
	}
	a.appender = nil

	return MoveBlocks(a.dir, a.destDir)
}

func (a *safeAppender) newBlockWriter(dir string, blockDuration int64) error {
	blockWriter, err := tsdb.NewBlockWriter(log.NewNopLogger(), dir, 2*blockDuration)
	if err != nil {
		return errors.Wrap(err, "failed to create block writer")
	}
	a.blockWriter = blockWriter
	appender := a.blockWriter.Appender(a.context)
	a.appender = &appender
	return nil
}

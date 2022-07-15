package database

import (
	"context"
	"crypto/rand"
	"fmt"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/tsdb"
	errors2 "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	DefaultBlockDuration = tsdb.DefaultBlockDuration
	DefaultRetention     = int64(90 * 24 * time.Hour / time.Millisecond)
)

type Database interface {
	AppendManager() (AppendManager, error)
	QueryManager() (QueryManager, error)
	GetBlockDuration() int64
	Compact() error
	Close() error
}

type database struct {
	context            context.Context
	mtx                *sync.RWMutex
	stopped            bool
	dir                string
	blockDuration      int64
	retentionPeriod    int64
	dbOnce             sync.Once
	dbError            error
	db                 *tsdb.DB
	appendManagerOnce  *sync.Once
	appendManagerError error
	appendManager      AppendManager
	queryManagerOnce   *sync.Once
	queryManagerError  error
	queryManager       QueryManager
}

func NewTempDirectory(dir string, extension string) (string, error) {
	uid := ulid.MustNew(ulid.Now(), rand.Reader)
	if err := deleteOldTempDirectories(dir, extension, uid.Time()); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s%s", dir, uid.String(), extension), nil
}

func deleteOldTempDirectories(dir string, extension string, time uint64) error {
	parent := filepath.Dir(dir)
	prefix := filepath.Base(dir) + "-"
	files, err := os.ReadDir(parent)
	if err != nil {
		return err
	}
	for _, f := range files {
		fn := f.Name()
		if f.IsDir() && strings.HasPrefix(fn, prefix) && filepath.Ext(fn) == extension {
			if id, errU := ulid.ParseStrict(fn[len(prefix) : len(fn)-len(extension)]); errU != nil {
				return errU
			} else if time > id.Time() {
				if err = os.RemoveAll(filepath.Join(parent, fn)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func MoveBlocks(sourceDir string, destDir string) error {
	if err := os.MkdirAll(destDir, 0o777); err != nil {
		return err
	}
	files, err := os.ReadDir(sourceDir)
	if err != nil {
		return err
	}
	for _, f := range files {
		if isBlockDir(f) {
			errR := fileutil.Replace(filepath.Join(sourceDir, f.Name()), filepath.Join(destDir, f.Name()))
			if errR != nil {
				return errR
			}
		}
	}
	return os.RemoveAll(sourceDir)
}

func isBlockDir(fi fs.DirEntry) bool {
	if !fi.IsDir() {
		return false
	}
	_, err := ulid.ParseStrict(fi.Name())
	return err == nil
}

func GetCompatibleBlockDuration(maxBlockDuration int64) int64 {
	blockDuration := DefaultBlockDuration
	if maxBlockDuration > DefaultBlockDuration {
		ranges := tsdb.ExponentialBlockRanges(DefaultBlockDuration, 10, 3)
		idx := len(ranges) - 1 // Use largest range if user asked for something enormous.
		for i, v := range ranges {
			if v > maxBlockDuration {
				idx = i - 1
				break
			}
		}
		blockDuration = ranges[idx]
	}
	return blockDuration
}

func NewDatabase(dir string, blockDuration int64, retentionPeriod int64, ctx context.Context) (Database, error) {
	mtx := new(sync.RWMutex)
	db := &database{
		context:           ctx,
		mtx:               mtx,
		dir:               dir,
		blockDuration:     blockDuration,
		retentionPeriod:   retentionPeriod,
		appendManagerOnce: new(sync.Once),
		queryManagerOnce:  new(sync.Once),
		stopped:           false,
	}
	return db, nil
}

func (d *database) AppendManager() (AppendManager, error) {
	appendManagerResetFunc := func() {
		d.appendManagerOnce = new(sync.Once)
		d.appendManager = nil
		d.appendManagerError = nil
	}
	d.appendManagerOnce.Do(func() {
		a, err := newAppendManager(d.context, d.mtx, d.dir, d.blockDuration, appendManagerResetFunc)
		if err != nil {
			d.appendManagerError = err
		}
		d.appendManager = a
	})
	return d.appendManager, d.appendManagerError
}

func (d *database) QueryManager() (QueryManager, error) {
	queryManagerResetFunc := func() {
		d.queryManagerOnce = new(sync.Once)
		d.queryManager = nil
		d.queryManagerError = nil
	}
	d.queryManagerOnce.Do(func() {
		db, err := d.openDatabase()
		if err != nil {
			d.queryManagerError = err
			d.queryManager = nil
			return
		}
		manager, err := newQueryManager(d.mtx, db, queryManagerResetFunc)
		if err != nil {
			d.queryManagerError = err
		}
		d.queryManager = manager
	})
	return d.queryManager, d.queryManagerError
}

func (d *database) GetBlockDuration() int64 {
	return d.blockDuration
}

func (d *database) Close() error {
	if d.stopped {
		return nil
	}
	d.stopped = true
	var err error
	if d.appendManager != nil {
		err = d.appendManager.Close()

	}
	if d.queryManager != nil {
		err2 := d.queryManager.Close()
		if err2 != nil {
			if err != nil {
				err = errors2.NewMulti(err, err2).Err()
			}
		}
	}
	return err
}

func (d *database) Compact() error {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	_, _ = fmt.Fprintf(os.Stderr, "Compacting data\n")

	if d.stopped {
		return errors.New("cannot compact a closed database")
	}
	db, err := d.openDatabase()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to compact database: %v\n", err)
		return err
	}
	err = db.Compact()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to compact database: %v\n", err)
		return err
	}
	return nil
}

func (d *database) openDatabase() (*tsdb.DB, error) {
	d.dbOnce.Do(func() {
		dbOptions := tsdb.DefaultOptions()
		dbOptions.AllowOverlappingBlocks = true
		dbOptions.EnableExemplarStorage = false
		dbOptions.EnableMemorySnapshotOnShutdown = false
		dbOptions.IsolationDisabled = false
		dbOptions.MaxBlockDuration = d.blockDuration
		dbOptions.MinBlockDuration = d.blockDuration
		dbOptions.NoLockfile = true
		dbOptions.RetentionDuration = d.retentionPeriod
		dbOptions.WALSegmentSize = -1

		registry := prometheus.NewRegistry()
		db, err := tsdb.Open(d.dir, nil, registry, dbOptions, nil)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to compact data: %v\n", err)
			d.dbError = err
		}
		d.db = db
		d.dbError = nil
	})
	return d.db, d.dbError
}

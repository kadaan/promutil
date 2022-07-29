package compactor

import (
	"context"
	"github.com/kadaan/promutil/config"
	"github.com/kadaan/promutil/lib/command"
	"github.com/kadaan/promutil/lib/database"
	"github.com/kadaan/promutil/lib/errors"
)

func NewCompactor() command.Task[config.CompactConfig] {
	return &compactor{}
}

type compactor struct {
}

func (t *compactor) Run(c *config.CompactConfig) error {
	db, err := database.NewDatabase(c.Directory, database.DefaultBlockDuration, database.DefaultRetention,
		context.Background())
	if err != nil {
		return errors.Wrap(err, "failed to open db")
	}
	defer func(db database.Database) {
		_ = db.Close()
	}(db)

	err = db.Compact()
	if err != nil {
		return errors.Wrap(err, "failed to compact")
	}
	return nil
}

package config

import (
	"fmt"
	"github.com/araddon/dateparse"
	"github.com/dustin/go-humanize"
	"github.com/kadaan/promutil/lib/errors"
	"github.com/tj/go-naturaldate"
	"strconv"
	"time"
)

var (
	Now = time.Now().UTC()
)

type timeValue time.Time

func NewTimeValue(p *time.Time, val time.Time) *timeValue {
	*p = val
	return (*timeValue)(p)
}

// String is used both by fmt.Print and by Cobra in help text
func (e *timeValue) String() string {
	return fmt.Sprintf("\"%s\"", humanize.Time((*time.Time)(e).UTC()))
}

// Set must have pointer receiver, so it doesn't change the value of a copy
func (e *timeValue) Set(v string) error {
	if i, err := strconv.ParseInt(v, 10, 64); err != nil {
		if t, err := naturaldate.Parse(v, Now, naturaldate.WithDirection(naturaldate.Past)); err != nil {
			if t, err := dateparse.ParseStrict(v); err != nil {
				return errors.Wrap(err, "cannot parse %q to a valid timestamp", v)
			} else {
				*e = timeValue(t.UTC())
				return nil
			}
		} else {
			*e = timeValue(t.UTC())
			return nil
		}
	} else {
		*e = timeValue(time.UnixMilli(i))
		return nil
	}
}

// Type is only used in help text
func (e *timeValue) Type() string {
	return "timestamp"
}

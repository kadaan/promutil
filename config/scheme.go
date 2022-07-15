package config

import (
	"github.com/pkg/errors"
)

type Scheme string

const (
	Http  Scheme = "http"
	Https Scheme = "https"
)

func NewSchemeValue(val Scheme, p *Scheme) *Scheme {
	*p = val
	return (*Scheme)(p)
}

// String is used both by fmt.Print and by Cobra in help text
func (e *Scheme) String() string {
	return string(*e)
}

// Set must have pointer receiver so it doesn't change the value of a copy
func (e *Scheme) Set(v string) error {
	switch v {
	case "http", "https":
		*e = Scheme(v)
		return nil
	default:
		return errors.New(`must be one of "http" or "https"`)
	}
}

// Type is only used in help text
func (e *Scheme) Type() string {
	return "scheme"
}

package config

import (
	"fmt"
	"github.com/kadaan/promutil/lib/errors"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
)

type matchersValue struct {
	value   *map[string][]*labels.Matcher
	changed bool
}

func NewMatchersValue(p *map[string][]*labels.Matcher) *matchersValue {
	rrv := new(matchersValue)
	rrv.value = p
	*rrv.value = make(map[string][]*labels.Matcher, 0)
	return rrv
}

// String is used both by fmt.Print and by Cobra in help text
func (e *matchersValue) String() string {
	size := len(*e.value)
	if size == 0 {
		return "None"
	}
	return fmt.Sprintf("%d matcher(s)", size)
}

// Set must have pointer receiver, so it doesn't change the value of a copy
func (e *matchersValue) Set(v string) error {
	matchers, err := parser.ParseMetricSelector(v)
	if err != nil {
		return errors.Wrap(err, "failed to parse matcher: %s", v)
	}
	(*e.value)[v] = matchers
	return nil
}

// Type is only used in help text
func (e *matchersValue) Type() string {
	return "matchers"
}

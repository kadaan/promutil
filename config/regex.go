package config

import (
	"bytes"
	"regexp"
	"strings"
)

type regexArrayValue struct {
	value   *[]*regexp.Regexp
	changed bool
}

func NewRegexValue(p *[]*regexp.Regexp, val []*regexp.Regexp) *regexArrayValue {
	rav := new(regexArrayValue)
	rav.value = p
	*rav.value = val
	return rav
}

// String is used both by fmt.Print and by Cobra in help text
func (e *regexArrayValue) String() string {
	b := &bytes.Buffer{}
	for _, r := range *e.value {
		b.WriteString(r.String())
		b.WriteString(",")
	}
	return strings.TrimSuffix(b.String(), ",")
}

// Set must have pointer receiver, so it doesn't change the value of a copy
func (e *regexArrayValue) Set(v string) error {
	regex, err := regexp.Compile(v)
	if err != nil {
		return err
	}
	if !e.changed {
		*e.value = []*regexp.Regexp{regex}
		e.changed = true
	} else {
		*e.value = append(*e.value, regex)
	}
	return nil
}

// Type is only used in help text
func (e *regexArrayValue) Type() string {
	return "regex"
}

package config

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/rules"
	errors2 "github.com/prometheus/prometheus/tsdb/errors"
)

type RecordingRules []*RecordingRule

type RecordingRule struct {
	rules.Rule
	Group string
}

type recordingRulesArrayValue struct {
	value   *RecordingRules
	changed bool
}

func NewRecordingRulesValue(p *RecordingRules) *recordingRulesArrayValue {
	rrv := new(recordingRulesArrayValue)
	rrv.value = p
	*rrv.value = make(RecordingRules, 0)
	return rrv
}

// String is used both by fmt.Print and by Cobra in help text
func (e *recordingRulesArrayValue) String() string {
	size := len(*e.value)
	if size == 0 {
		return "None"
	}
	return fmt.Sprintf("%d recording rules", size)
}

// Set must have pointer receiver, so it doesn't change the value of a copy
func (e *recordingRulesArrayValue) Set(v string) error {
	rgs, errs := rulefmt.ParseFile(v)
	if errs != nil {
		return errors.Wrap(errors2.NewMulti(errs...).Err(),
			fmt.Sprintf("failed to parse recording rule file '%s'", v))
	}
	for _, rg := range rgs.Groups {
		for _, rule := range rg.Rules {
			if rule.Record.Value != "" {
				expr, err := parser.ParseExpr(rule.Expr.Value)
				if err != nil {
					return errors.Wrap(err,
						fmt.Sprintf("failed to parse recording rule expression '%s'", rule.Expr.Value))
				}
				recordingRule := &RecordingRule{
					Rule: rules.NewRecordingRule(
						rule.Record.Value,
						expr,
						labels.FromMap(rule.Labels),
					),
					Group: rg.Name,
				}
				if !e.changed {
					*e.value = make(RecordingRules, 0)
					e.changed = true
				}
				*e.value = append(*e.value, recordingRule)
			}
		}
	}
	return nil
}

// Type is only used in help text
func (e *recordingRulesArrayValue) Type() string {
	return "recordingRules"
}

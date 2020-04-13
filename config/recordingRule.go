package config

import (
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/promql/parser"
)

type RecordingRules []*RecordingRule

type RecordingRule struct {
	Name       string
	Expression parser.Expr
	Labels     labels.Labels
}
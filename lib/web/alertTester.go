package web

import (
	"bytes"
	"context"
	"fmt"
	"github.com/cespare/xxhash/v2"
	"github.com/gin-gonic/gin"
	kitLog "github.com/go-kit/kit/log"
	"github.com/kadaan/promutil/config"
	"github.com/kadaan/promutil/lib/common"
	"github.com/kadaan/promutil/lib/errors"
	"github.com/kadaan/promutil/lib/remote"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/util/stats"
	"net/http"
	"net/url"
	"strings"
	textTemplate "text/template"
	"time"
)

const (
	alertTesterLinkName     = "Alert Tester"
	alertTestingRoute       = "/alerts_testing"
	alertRuleTestingRoute   = "/alert-rule-testing"
	alertForStateMetricName = "ALERTS_FOR_STATE"
	alertHtmlSnippet        = `name: {{ .Alert }}
expr: {{ .Expr }}
for: {{ .For }}
{{- if .Labels }}
labels:
  {{- range $key, $value := .Labels }}
    {{ $key }}: {{ $value }}
  {{- end }}
{{- end }}
{{- if .Annotations }}
annotations:
  {{- range $key, $value := .Annotations }}
    {{ $key }}: {{ $value }}
  {{- end }}
{{- end }}`
)

type timestamp int64

func (t timestamp) MarshalJSON() ([]byte, error) {
	buffer := bytes.Buffer{}
	ts := int64(t)
	if ts < 0 {
		buffer.WriteString("-")
		ts = -ts
	}
	buffer.WriteString(fmt.Sprintf("%d", ts/1000))
	fraction := ts % 1000
	if fraction != 0 {
		buffer.WriteString(".")
		if fraction < 100 {
			buffer.WriteString("0")
		}
		if fraction < 10 {
			buffer.WriteString("0")
		}
		buffer.WriteString(fmt.Sprintf("%d", fraction))
	}
	return buffer.Bytes(), nil
}

type alertsTestResult struct {
	IsError              bool                        `json:"isError"`
	Errors               []string                    `json:"errors"`
	Start                timestamp                   `json:"start"`
	End                  timestamp                   `json:"end"`
	Step                 int64                       `json:"step"`
	AlertStateToRowClass map[rules.AlertState]string `json:"alertStateToRowClass"`
	AlertStateToName     map[rules.AlertState]string `json:"alertStateToName"`
	RuleResults          []ruleResult                `json:"ruleResults,omitempty"`
}

func (r *alertsTestResult) addErrors(errors ...error) {
	if len(errors) > 0 {
		for _, err := range errors {
			if err != nil {
				r.IsError = true
				r.Errors = append(r.Errors, err.Error())
			}
		}
	}
}

func newAlertsTestResult() alertsTestResult {
	return alertsTestResult{
		AlertStateToRowClass: map[rules.AlertState]string{
			rules.StateInactive: "success",
			rules.StatePending:  "warning",
			rules.StateFiring:   "danger",
		},
		AlertStateToName: map[rules.AlertState]string{
			rules.StateInactive: strings.ToUpper(rules.StateInactive.String()),
			rules.StatePending:  strings.ToUpper(rules.StatePending.String()),
			rules.StateFiring:   strings.ToUpper(rules.StateFiring.String()),
		},
	}
}

type ruleResult struct {
	Group           string             `json:"group"`
	Name            string             `json:"name"`
	Alerts          *[]rules.Alert     `json:"alerts"`
	MatrixResult    *queryData         `json:"matrixResult"`
	ExprQueryResult *queryDataWithExpr `json:"exprQueryResult"`
	HTMLSnippet     string             `json:"htmlSnippet"`
}

type queryData struct {
	ResultType parser.ValueType  `json:"resultType"`
	Result     parser.Value      `json:"result"`
	Stats      *stats.QueryStats `json:"stats"`
}

type queryDataWithExpr struct {
	ResultType parser.ValueType `json:"resultType"`
	Result     parser.Value     `json:"result"`
	Expr       string           `json:"expr"`
}

type alertTester struct {
	templateExecutor         TemplateExecutor
	queryable                remote.Queryable
	config                   *config.WebConfig
	alertHtmlSnippetTemplate *textTemplate.Template
}

func NewAlertTester(config *config.WebConfig) (Route, error) {
	alertHtmlSnippetTemplate := textTemplate.New("alertHtmlSnippet")
	var err error
	if alertHtmlSnippetTemplate, err = alertHtmlSnippetTemplate.Parse(alertHtmlSnippet); err != nil {
		return nil, errors.Wrap(err, "failed to parse alert html snippet")
	}
	return &alertTester{
		config:                   config,
		alertHtmlSnippetTemplate: alertHtmlSnippetTemplate,
	}, nil
}

func (t *alertTester) GetOrder() int {
	return 0
}

func (t *alertTester) GetDefault() *string {
	defaultRoute := alertRuleTestingRoute
	return &defaultRoute
}

func (t *alertTester) GetNavBarLinks() []NavBarLink {
	return []NavBarLink{{
		Path: alertRuleTestingRoute,
		Name: alertTesterLinkName,
	}}
}

func (t *alertTester) Register(router gin.IRouter, templateExecutor TemplateExecutor, queryable remote.Queryable) {
	t.templateExecutor = templateExecutor
	t.queryable = queryable
	router.GET(alertRuleTestingRoute, t.alertRuleTesting)
	router.POST(alertTestingRoute, t.alertsTesting)
}

func (t *alertTester) alertRuleTesting(requestContext *gin.Context) {
	t.templateExecutor.ExecuteTemplate(requestContext, "alert-rule-testing.html")
}

func (t *alertTester) alertsTesting(requestContext *gin.Context) {
	result := newAlertsTestResult()
	if cfg, err := t.parseAlertsTestingBody(requestContext.Request); err != nil {
		result.addErrors(err)
	} else {
		result.Start = timestamp(cfg.Start.UnixMilli())
		result.End = timestamp(cfg.End.UnixMilli())
		result.Step = cfg.Step.Milliseconds()
		for _, group := range cfg.RuleGroups.Groups {
			for _, rule := range group.Rules {
				if rule.Alert.Value == "" {
					continue
				}
				htmlSnippet, alerts, matrixResult, exprQueryResult, errA := t.evaluateAlertRule(
					requestContext.Request.Context(),
					t.queryable,
					cfg.Start,
					cfg.End,
					cfg.Step,
					group,
					rule)
				r := ruleResult{
					Group:           group.Name,
					Name:            rule.Alert.Value,
					Alerts:          alerts,
					HTMLSnippet:     htmlSnippet,
					MatrixResult:    matrixResult,
					ExprQueryResult: exprQueryResult,
				}
				result.addErrors(errA)
				result.RuleResults = append(result.RuleResults, r)
			}
		}
	}
	requestContext.JSON(200, result)
}

type alertsTestingConfig struct {
	Start      time.Time
	End        time.Time
	Step       time.Duration
	RuleGroups *rulefmt.RuleGroups
}

func (t *alertTester) parseAlertsTestingBody(r *http.Request) (*alertsTestingConfig, error) {
	configString := r.FormValue("config")
	if configString == "" {
		return nil, errors.New("failed to parse alert testing request: missing alert config text")
	}
	end, err := common.ParseTime(r.FormValue("end"))
	if err != nil {
		return nil, errors.New("failed to parse alert testing request: could not parse end time")
	}
	start := end.Add(-24 * time.Hour)
	if startRaw := r.FormValue("start"); startRaw != "" {
		start, err = common.ParseTime(startRaw)
		if err != nil {
			return nil, errors.New("failed to parse alert testing request: could not parse start time")
		}
	}

	step, _ := common.ParseDuration("15s")
	if stepRaw := r.FormValue("step"); stepRaw != "" {
		step, err = common.ParseDuration(stepRaw)
		if err != nil {
			return nil, errors.New("failed to parse alert testing request: could not parse step duration")
		}
		if step <= 0 {
			return nil, errors.New("failed to parse alert testing request: step duration cannot be <= 0")
		}
	}

	// For safety, limit the number of returned points per timeseries.
	// This is sufficient for 60s resolution for a week or 1h resolution for a year.
	if end.Sub(start)/step > 11000 {
		return nil, errors.New("failed to parse alert testing request: exceeded maximum resolution of 11,000 points")
	}

	configStringUnescaped, err := url.QueryUnescape(configString)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse alert testing request: could not unescape rule config")
	}

	ruleGroups, errs := rulefmt.Parse([]byte(configStringUnescaped))
	if len(errs) > 0 {
		return nil, errors.NewMulti(errs, "failed to parse alert testing request: could not parse rule config")
	}

	return &alertsTestingConfig{
		Start:      start,
		End:        end,
		Step:       step,
		RuleGroups: ruleGroups,
	}, nil
}

func (t *alertTester) evaluateAlertRule(ctx context.Context, queryable remote.Queryable, minTimestamp time.Time, maxTimestamp time.Time, step time.Duration, group rulefmt.RuleGroup, rule rulefmt.RuleNode) (string, *[]rules.Alert, *queryData, *queryDataWithExpr, error) {
	expr, err := parser.ParseExpr(rule.Expr.Value)
	if err != nil {
		return "", nil, nil, nil, errors.Wrap(err, "failed to parse the expression %q", rule.Expr)
	}
	interval := time.Duration(group.Interval)
	if interval <= 0 {
		interval = 15 * time.Second
	}
	alertingRule := rules.NewAlertingRule(
		rule.Alert.Value,
		expr,
		time.Duration(rule.For),
		labels.FromMap(rule.Labels),
		labels.FromMap(rule.Annotations),
		labels.Labels{},
		"",
		true,
		kitLog.NewNopLogger(),
	)

	provider, err := queryable.QueryFuncProvider(minTimestamp, maxTimestamp, interval)
	if err != nil {
		return "", nil, nil, nil, errors.Wrap(err, "failed to create queryable")
	}

	maxSamples := int((maxTimestamp.UnixMilli() - minTimestamp.UnixMilli()) / step.Milliseconds())

	rangeQueryFunc := provider.RangeQueryFunc()
	queryMatrix, err := rangeQueryFunc(ctx,
		alertingRule.Query().String(),
		minTimestamp,
		maxTimestamp,
		interval)
	if err != nil {
		return "", nil, nil, nil, errors.Wrap(err, "failed to query %s from %d to %d", rule.Expr.Value, minTimestamp, maxTimestamp)
	}

	queryMatrix = common.DownsampleMatrix(queryMatrix, maxSamples, true)

	activeAlertsByLabels := make(map[uint64][]*rules.Alert)

	queryFunc := provider.InstantQueryFunc(false)
	seriesHashMap := make(map[uint64]*promql.Series)
	nextIter := func(curr, max time.Time) time.Time {
		diff := max.Sub(curr)
		if diff != 0 && diff < interval {
			return max
		}
		return curr.Add(interval)
	}
	for ts := minTimestamp; maxTimestamp.Sub(ts) >= 0; ts = nextIter(ts, maxTimestamp) {
		vec, errA := alertingRule.Eval(
			ctx,
			ts,
			queryFunc,
			nil,
			group.Limit)
		if errA != nil {
			return "", nil, nil, nil, errors.Wrap(errA, "failed to evaluate rule %s at %d", rule.Expr.Value, ts)
		}
		for _, smpl := range vec {
			series, ok := seriesHashMap[smpl.Metric.Hash()]
			if !ok {
				series = &promql.Series{Metric: smpl.Metric}
				seriesHashMap[smpl.Metric.Hash()] = series
			}
			series.Points = append(series.Points, smpl.Point)
		}
		alertingRule.ForEachActiveAlert(func(activeAlert *rules.Alert) {
			aaHash := t.activeAlertHash(activeAlert)
			if existingAlerts, exists := activeAlertsByLabels[aaHash]; !exists {
				anew := *activeAlert
				activeAlertsByLabels[aaHash] = make([]*rules.Alert, 0)
				activeAlertsByLabels[aaHash] = append(activeAlertsByLabels[aaHash], &anew)
			} else {
				shouldAdd := true
				for index, existingAlert := range existingAlerts {
					if existingAlert.ResolvedAt.IsZero() && !activeAlert.ResolvedAt.IsZero() && existingAlert.State == rules.StateFiring && activeAlert.State == rules.StateInactive {
						existingAlert.ResolvedAt = activeAlert.ResolvedAt
						shouldAdd = false
						break
					}
					if existingAlert.ActiveAt == activeAlert.ActiveAt && existingAlert.State == rules.StatePending && activeAlert.State == rules.StateFiring {
						anew := *activeAlert
						existingAlerts[index] = &anew
						shouldAdd = false
						break
					}
					if existingAlert.ActiveAt == activeAlert.ActiveAt && existingAlert.FiredAt == activeAlert.FiredAt && activeAlert.State == existingAlert.State {
						shouldAdd = false
						break
					}
					if activeAlert.State == rules.StateInactive {
						shouldAdd = false
						break
					}
				}
				if shouldAdd {
					anew := *activeAlert
					activeAlertsByLabels[aaHash] = append(activeAlertsByLabels[aaHash], &anew)
				}
			}
		})
	}

	var matrix promql.Matrix
	for _, series := range seriesHashMap {
		if series.Metric.Get(labels.MetricName) == alertForStateMetricName {
			continue
		}
		p := 0
		for p < len(matrix) {
			if matrix[p].Metric.Hash() >= series.Metric.Hash() {
				break
			}
			p++
		}
		matrix = append(matrix[:p], append(promql.Matrix{*series}, matrix[p:]...)...)
	}

	matrix = common.DownsampleMatrix(matrix, maxSamples, false)

	htmlSnippet, err := t.htmlSnippetWithoutLinks(alertingRule)
	if err != nil {
		return "", nil, nil, nil, err
	}

	var activeAlertList []rules.Alert
	alertQueryFunc := provider.InstantQueryFunc(true)
	for _, activeAlertsByLabel := range activeAlertsByLabels {
		for _, activeAlert := range activeAlertsByLabel {
			var ts time.Time
			if activeAlert.State == rules.StatePending {
				ts = activeAlert.ActiveAt
			} else {
				ts = activeAlert.FiredAt
			}
			alertingRule = rules.NewAlertingRule(
				rule.Alert.Value,
				expr,
				time.Duration(rule.For),
				labels.FromMap(rule.Labels),
				labels.FromMap(rule.Annotations),
				labels.Labels{},
				"",
				true,
				kitLog.NewNopLogger(),
			)
			_, errA := alertingRule.Eval(
				ctx,
				ts,
				alertQueryFunc,
				nil,
				group.Limit)
			if errA != nil {
				return "", nil, nil, nil, errors.Wrap(errA, "failed to evaluate rule %s at %d", rule.Expr.Value, ts)
			}
			alertingRule.ForEachActiveAlert(func(alert *rules.Alert) {
				if alert.ActiveAt == ts && activeAlert.Labels.Hash() == alert.Labels.Hash() {
					(*activeAlert).Annotations = (*alert).Annotations
				}
			})
			activeAlertList = append(activeAlertList, *activeAlert)
		}
	}

	return htmlSnippet,
		&activeAlertList,
		&queryData{
			Result:     matrix,
			ResultType: matrix.Type(),
		},
		&queryDataWithExpr{
			Result:     queryMatrix,
			ResultType: queryMatrix.Type(),
			Expr:       rule.Expr.Value,
		},
		nil
}

func (t *alertTester) activeAlertHash(alert *rules.Alert) uint64 {
	var buf []byte
	buf = append(buf, fmt.Sprintf("%d", alert.Labels.Hash())...)
	buf = append(buf, fmt.Sprintf("%d", alert.ActiveAt.UnixNano())...)
	return xxhash.Sum64(buf)
}

func (t *alertTester) htmlSnippetWithoutLinks(r *rules.AlertingRule) (string, error) {
	lbls := make(map[string]string, len(r.Labels()))
	for _, l := range r.Labels() {
		lbls[l.Name] = l.Value
	}
	annotations := make(map[string]string, len(r.Annotations()))
	for _, l := range r.Annotations() {
		annotations[l.Name] = l.Value
	}
	ar := rulefmt.Rule{
		Alert:       r.Name(),
		Expr:        r.Query().String(),
		For:         model.Duration(r.HoldDuration()),
		Labels:      lbls,
		Annotations: annotations,
	}

	var tpl bytes.Buffer
	if err := t.alertHtmlSnippetTemplate.Execute(&tpl, ar); err != nil {
		return "", errors.Wrap(err, "failed to execute alert html snippet template")
	}
	return tpl.String(), nil
}

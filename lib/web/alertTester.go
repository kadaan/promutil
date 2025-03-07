package web

import (
	"context"
	"fmt"
	"github.com/cespare/xxhash/v2"
	"github.com/gin-gonic/gin"
	kitLog "github.com/go-kit/kit/log"
	jsoniter "github.com/json-iterator/go"
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
	htmlTemplate "html/template"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

const (
	alertTesterLinkName     = "Alert Tester"
	alertTestingRoute       = "/alerts_testing"
	alertRuleTestingRoute   = "/alert-rule-testing"
	alertForStateMetricName = "ALERTS_FOR_STATE"
)

func init() {
	jsoniter.RegisterTypeEncoderFunc("float64", marshalValueJSON, marshalValueJSONIsEmpty)
	jsoniter.RegisterTypeEncoderFunc("time.Time", marshalTimeJSON, marshalTimeJSONIsEmpty)
	jsoniter.RegisterTypeEncoderFunc("promql.Point", marshalPointJSON, marshalPointJSONIsEmpty)
}

func marshalPointJSON(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	p := *((*promql.Point)(ptr))
	stream.WriteArrayStart()
	marshalTimestamp(p.T, stream)
	stream.WriteMore()
	marshalValue(p.V, stream)
	stream.WriteArrayEnd()
}

func marshalPointJSONIsEmpty(_ unsafe.Pointer) bool {
	return false
}

func marshalTimeJSON(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	p := *((*time.Time)(ptr))
	if p.IsZero() {
		stream.WriteNil()
	} else {
		marshalTimestamp(p.UnixMilli(), stream)
	}
}

func marshalTimeJSONIsEmpty(_ unsafe.Pointer) bool {
	return false
}

func marshalTimestamp(t int64, stream *jsoniter.Stream) {
	// Write out the timestamp as a float divided by 1000.
	// This is ~3x faster than converting to a float.
	if t < 0 {
		stream.WriteRaw(`-`)
		t = -t
	}
	stream.WriteInt64(t / 1000)
	fraction := t % 1000
	if fraction != 0 {
		stream.WriteRaw(`.`)
		if fraction < 100 {
			stream.WriteRaw(`0`)
		}
		if fraction < 10 {
			stream.WriteRaw(`0`)
		}
		stream.WriteInt64(fraction)
	}
}

func marshalValueJSON(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	p := *((*float64)(ptr))
	marshalValue(p, stream)
}

func marshalValueJSONIsEmpty(_ unsafe.Pointer) bool {
	return false
}

func marshalValue(v float64, stream *jsoniter.Stream) {
	if math.IsNaN(v) {
		stream.WriteString("NaN")
	} else {
		stream.WriteRaw(`"`)
		// Taken from https://github.com/json-iterator/go/blob/master/stream_float.go#L71 as a workaround
		// to https://github.com/json-iterator/go/issues/365 (jsoniter, to follow json standard, doesn't allow inf/nan).
		buf := stream.Buffer()
		abs := math.Abs(v)
		valueFmt := byte('f')
		// Note: Must use float32 comparisons for underlying float32 value to get precise cutoffs right.
		if abs != 0 {
			if abs < 1e-6 || abs >= 1e21 {
				valueFmt = 'e'
			}
		}
		buf = strconv.AppendFloat(buf, v, valueFmt, -1, 64)
		stream.SetBuffer(buf)
		stream.WriteRaw(`"`)
	}
}

type alertsTestResult struct {
	IsError              bool                        `json:"isError"`
	Errors               []string                    `json:"errors"`
	Start                time.Time                   `json:"start"`
	End                  time.Time                   `json:"end"`
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
	Definition      *alertDefinition   `json:"definition"`
	Alerts          *[]rules.Alert     `json:"alerts"`
	MatrixResult    *queryData         `json:"matrixResult"`
	ExprQueryResult *queryDataWithExpr `json:"exprQueryResult"`
}

type alertDefinition struct {
	Group        string              `json:"group"`
	Name         string              `json:"name"`
	Expr         string              `json:"expr"`
	ExprTableUrl string              `json:"exprTableUrl"`
	For          string              `json:"for"`
	Labels       []map[string]string `json:"labels"`
	Annotations  []map[string]string `json:"annotations"`
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
	templateExecutor TemplateExecutor
	queryable        remote.Queryable
	config           *config.WebConfig
	//alertHtmlSnippetTemplate *textTemplate.Template
}

func NewAlertTester(config *config.WebConfig) (Route, error) {
	//alertHtmlSnippetTemplate := textTemplate.New("alertHtmlSnippet")
	//var err error
	//if alertHtmlSnippetTemplate, err = alertHtmlSnippetTemplate.Parse(alertHtmlSnippet); err != nil {
	//	return nil, errors.Wrap(err, "failed to parse alert html snippet")
	//}
	return &alertTester{
		config: config,
		//alertHtmlSnippetTemplate: alertHtmlSnippetTemplate,
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
		result.Start = time.UnixMilli(cfg.Start.UnixMilli())
		result.End = time.UnixMilli(cfg.End.UnixMilli())
		result.Step = cfg.Step.Milliseconds()
		for _, group := range cfg.RuleGroups.Groups {
			for _, rule := range group.Rules {
				if rule.Alert.Value == "" {
					continue
				}
				alertDefinition, alerts, matrixResult, exprQueryResult, errA := t.evaluateAlertRule(
					requestContext.Request.Context(),
					t.queryable,
					cfg.Start,
					cfg.End,
					cfg.Step,
					group,
					rule)
				r := ruleResult{
					Definition:      alertDefinition,
					Alerts:          alerts,
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

func (t *alertTester) evaluateAlertRule(ctx context.Context, queryable remote.Queryable, minTimestamp time.Time, maxTimestamp time.Time, step time.Duration, group rulefmt.RuleGroup, rule rulefmt.RuleNode) (*alertDefinition, *[]rules.Alert, *queryData, *queryDataWithExpr, error) {
	expr, err := parser.ParseExpr(rule.Expr.Value)
	if err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "failed to parse the expression %q", rule.Expr)
	}
	interval := time.Duration(group.Interval)
	if interval <= 0 {
		interval = 15 * time.Second
	}
	alertingRule := rules.NewAlertingRule(
		rule.Alert.Value,
		expr,
		time.Duration(rule.For),
		labels.Labels{},
		labels.Labels{},
		labels.Labels{},
		"",
		true,
		kitLog.NewNopLogger(),
	)

	provider, err := queryable.QueryFuncProvider(minTimestamp, maxTimestamp, interval)
	if err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "failed to create queryable")
	}

	maxSamples := int((maxTimestamp.UnixMilli() - minTimestamp.UnixMilli()) / step.Milliseconds())

	rangeQueryFunc := provider.RangeQueryFunc()
	queryMatrix, err := rangeQueryFunc(ctx,
		alertingRule.Query().String(),
		minTimestamp,
		maxTimestamp,
		interval)
	if err != nil {
		return nil, nil, nil, nil, errors.Wrap(err, "failed to query %s from %d to %d", rule.Expr.Value, minTimestamp, maxTimestamp)
	}

	queryMatrix = common.DownsampleMatrix(queryMatrix, maxSamples, true)

	importantAlertTimestampSet := make(map[time.Time]interface{})

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
			return nil, nil, nil, nil, errors.Wrap(errA, "failed to evaluate rule %s at %d", rule.Expr.Value, ts)
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
			if !activeAlert.ActiveAt.IsZero() {
				importantAlertTimestampSet[activeAlert.ActiveAt] = nil
			}
			if !activeAlert.FiredAt.IsZero() {
				importantAlertTimestampSet[activeAlert.FiredAt] = nil
			}
			if !activeAlert.ResolvedAt.IsZero() {
				importantAlertTimestampSet[activeAlert.ResolvedAt] = nil
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

	var importantAlertTimestamps []time.Time
	for ts := range importantAlertTimestampSet {
		importantAlertTimestamps = append(importantAlertTimestamps, ts)
	}
	sort.Slice(importantAlertTimestamps, func(i, j int) bool {
		return importantAlertTimestamps[i].Before(importantAlertTimestamps[j])
	})

	activeAlertsByLabels := make(map[uint64][]*rules.Alert)
	alertQueryFunc := provider.InstantQueryFunc(true)
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
	for _, ts := range importantAlertTimestamps {
		_, errA := alertingRule.Eval(
			ctx,
			ts,
			alertQueryFunc,
			nil,
			group.Limit)
		if errA != nil {
			return nil, nil, nil, nil, errors.Wrap(errA, "failed to evaluate rule %s at %d", rule.Expr.Value, ts)
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

	var activeAlertList []rules.Alert
	for _, activeAlertsByLabel := range activeAlertsByLabels {
		for _, activeAlert := range activeAlertsByLabel {
			activeAlertList = append(activeAlertList, *activeAlert)
		}
	}
	sort.Slice(activeAlertList, func(i, j int) bool {
		return activeAlertList[i].ActiveAt.Before(activeAlertList[j].ActiveAt)
	})

	return t.toAlertDefinition(group, alertingRule, minTimestamp, maxTimestamp),
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

func (t *alertTester) toAlertDefinition(group rulefmt.RuleGroup, rule *rules.AlertingRule, startTime time.Time, endTime time.Time) *alertDefinition {
	lbls := make([]map[string]string, len(rule.Labels()))
	for i, l := range rule.Labels() {
		lbls[i] = map[string]string{"name": l.Name, "value": l.Value}
	}
	annotations := make([]map[string]string, len(rule.Annotations()))
	for i, a := range rule.Annotations() {
		annotations[i] = map[string]string{"name": a.Name, "value": a.Value}
	}
	return &alertDefinition{
		Group:        group.Name,
		Name:         rule.Name(),
		Expr:         rule.Query().String(),
		ExprTableUrl: t.graphLinkForExpression(rule.Query().String(), startTime, endTime),
		For:          model.Duration(rule.HoldDuration()).String(),
		Labels:       lbls,
		Annotations:  annotations,
	}
}

func (t *alertTester) graphLinkForExpression(expr string, startTime time.Time, endTime time.Time) string {
	escapedExpression := url.QueryEscape(expr)
	return fmt.Sprintf("%s/graph?g0.expr=%s&g0.tab=0&g0.range_input=%s&g0.end_input=%s&g0.moment_input=%s",
		t.config.Host.String(),
		escapedExpression,
		model.Duration(endTime.Sub(startTime)).String(),
		htmlTemplate.HTMLEscapeString(startTime.Format("2006-01-02 15:04:05")),
		htmlTemplate.HTMLEscapeString(endTime.Format("2006-01-02 15:04:05")))
}

let graphTemplate;
let popupTemplate;
let alertStateToRowClass;
let alertStateToName;
let endDate = null;

function escapeHTML(string) {
    const entityMap = {
        "&": "&amp;",
        "<": "&lt;",
        ">": "&gt;",
        '"': '&quot;',
        "'": '&#39;',
        "/": '&#x2F;'
    };

    return String(string).replace(/[&<>"'\/]/g, function (s) {
        return entityMap[s];
    });
}

function mustacheFormatMap(map) {
    const formatted = []
    for (const key in map) {
        formatted.push({
            'key': key,
            'value': map[key]
        })
    }
    return formatted
}

function isTimestampZero(timestamp) {
    if (timestamp === null) {
        return true;
    }
    return moment.utc(new Date(timestamp)).unix() === 0;
}

function mustacheFormatDate(timestamp) {
    if (timestamp === null) {
        return "";
    }
    const date = moment.utc(new Date(timestamp));
    if (date.unix() === 0) {
        return "";
    }
    return date.format('YYYY-MM-DD HH:mm:ss[Z]');
}

/**
 * Control
 */

const Control = function(options) {
    this.options = options;

    this.initialize();
}

Control.timeFactors = {
    "y": 60 * 60 * 24 * 365,
    "w": 60 * 60 * 24 * 7,
    "d": 60 * 60 * 24,
    "h": 60 * 60,
    "m": 60,
    "s": 1
};

Control.stepValues = [
    "1s", "10s", "1m", "5m", "15m", "30m", "1h", "2h", "6h", "12h", "1d", "2d", "1w", "2w", "4w", "8w", "1y", "2y"
];

Control.prototype.annotationsShown = function() {
    const self = this;
    return !$(self.showAnnotations).children('i').hasClass('glyphicon-unchecked');
}

Control.prototype.graphsShown = function() {
    const self = this;
    return !$(self.showGraphs).children('i').hasClass('glyphicon-unchecked');
}

Control.prototype.initialize = function() {
    const self = this;

    if (self.options.tab === undefined) {
        self.options.tab = 1;
    }

    const options = {
        'end': endDate,
    };

    jQuery.extend(options, self.options);

    // Get references to all the interesting elements in the control container and
    // bind event handlers.
    const controlWrapper = $("#control_wrapper");
    self.endDate = controlWrapper.find("input[name=end_input]");
    self.endDate.datetimepicker({
        locale: 'en',
        format: 'YYYY-MM-DD HH:mm',
        toolbarPlacement: 'bottom',
        sideBySide: true,
        showTodayButton: true,
        showClear: true,
        showClose: true,
        timeZone: 'UTC',
    });

    if (self.options.end_input) {
        self.endDate.data('DateTimePicker').date(self.options.end_input);
    }

    self.rangeInput = controlWrapper.find("input[name=range_input]");

    controlWrapper.find("button[name=inc_range]").click(function() { self.increaseRange(); });
    controlWrapper.find("button[name=dec_range]").click(function() { self.decreaseRange(); });

    controlWrapper.find("button[name=inc_end]").click(function() { self.increaseEnd(); });
    controlWrapper.find("button[name=dec_end]").click(function() { self.decreaseEnd(); });

    self.alertRangeInfo = $(".alert_range_info").hide();
    self.alertRangeInfoStart = self.alertRangeInfo.find("#alert_range_info_start");
    self.alertRangeInfoEnd = self.alertRangeInfo.find("#alert_range_info_end");
    self.alertRangeInfoStep = self.alertRangeInfo.find("#alert_range_info_step");

    self.evaluateBtn = controlWrapper.find(".evaluate");
    self.evaluateBtn.click(function() {
        self.evaluate();
    });

    self.alertHeader = $(".alert_header");
    self.showAnnotations = $("div.show-annotations");
    self.showGraphs = $("div.show-graphs");

    self.alertHeader.off();
    self.showAnnotations.off();
    self.showGraphs.off();

    self.alertHeader.click(function() {
        $(this).next().toggle();
    });

    self.showAnnotations.click(function() {
        if (self.annotationsShown()) {
            $(".alert_annotations").hide();
            $(".alert_annotations_header").hide();
            $(self.showAnnotations).children('i').removeClass('glyphicon-check').addClass('glyphicon-unchecked');
            self.showAnnotations.removeClass('is-checked');
        } else {
            $(".alert_annotations").show();
            $(".alert_annotations_header").show();
            $(self.showAnnotations).children('i').removeClass('glyphicon-unchecked').addClass('glyphicon-check');
            self.showAnnotations.addClass('is-checked');
        }
    });

    self.showGraphs.click(function() {
        if (self.graphsShown()) {
            $(".graph_header").hide();
            $(".graph_body").hide();
            $(self.showGraphs).children('i').removeClass('glyphicon-check').addClass('glyphicon-unchecked');
            self.showGraphs.removeClass('is-checked');
        } else {
            $(".graph_header").show();
            $(".graph_body").show();
            $(self.showGraphs).children('i').removeClass('glyphicon-unchecked').addClass('glyphicon-check');
            self.showGraphs.addClass('is-checked');
            for (let i in self.graphs) {
                self.graphs[i].resizeGraph();
            }
        }
    });

    self.graphs = [];
}

Control.prototype.evaluate = function() {
    const self = this;
    self.alertRangeInfo.hide();
    const endTime = self.getOrSetEndDate().valueOf() / 1000; // TODO: shouldn't valueof only work when it's a moment?
    const rangeSeconds = self.parseDuration(self.rangeInput.val()) * 1000;
    const startTime = endTime - rangeSeconds / 1000;
    const resolution = self.parseDuration("15s");
    const text = ace.edit("ruleTextArea").getValue();
    const data = {
        config: encodeURIComponent(text),
        start: startTime,
        end: endTime,
        step: resolution,
    };
    self.doEvaluate(data);
}

Control.prototype.doEvaluate = function(data) {
    const self = this;
    const time = data.end * 1000;
    if (time === 0) {
        $("#ruleTestInfo").html("Testing for current time");
        $(".evaluation_message").html("Testing for current time");
    } else {
        $("#ruleTestInfo").html("Testing for: " + mustacheFormatDate(time));
        $(".evaluation_message").html("Testing for: " + mustacheFormatDate(time));
    }
    $.ajax({
        method: 'POST',
        url: PATH_PREFIX + "/alerts_testing",
        dataType: "json",
        contentType: "application/x-www-form-urlencoded",
        data: $.param(data),
        success: function(json) {
            if (json.isError) {
                let errStr = "Error message:<br/>"
                const len = json.errors.length
                for(let i = 0; i < len; i++) {
                    errStr += "(" + (i+1) + ") " + json.errors[i] + '<br/>'
                }
                $("#ruleTestInfo").html(redHtml(errStr));
            } else {
                if (time === 0) {
                    endDate = null;
                } else {
                    endDate = new Date(time);
                }
                $("#ruleTestInfo").html(greenHtml("Evaluated"));
                $(".evaluation_message").html("");
                alertStateToRowClass = json.alertStateToRowClass;
                alertStateToName = json.alertStateToName;
            }
            self.replaceRules(json);
        },
        error: function(jqXHR, textStatus, errorThrown) {
            let errorMessage = errorThrown;
            if ((errorMessage ?? "") === "") {
                errorMessage = "Cannot connect to Promutil API";
            }
            $("#ruleTestInfo").html(redHtml("ERROR: "+errorMessage));
        }
    });
}

Control.prototype.replaceRules = function(json) {
    const self = this;
    const graphContainer = $("#graph_container");
    graphContainer.empty();
    Graph.numGraphs = 0;

    self.alertRangeInfoStart.text(mustacheFormatDate(json.start*1000));
    self.alertRangeInfoEnd.text(mustacheFormatDate(json.end*1000));
    self.alertRangeInfoStep.text(moment.duration(json.step).humanize());
    self.alertRangeInfo.show();

    const graphs = [];
    const areGraphsShown = self.graphsShown();
    const areAnnotationsShown = self.annotationsShown();
    for(let i in json.ruleResults) {
        const graph = new Graph(
            graphContainer,
            {
                start: json.start,
                end: json.end,
                step: json.step,
                data: json.ruleResults[i]
            },
            areGraphsShown,
            areAnnotationsShown
        );
        graphs.push(graph);
        $(window).resize(function() {
            graph.resizeGraph();
        });
    }

    self.graphs = graphs;
    $('[data-toggle="popover"]').popover();
};

Control.prototype.increaseRange = function() {
    const self = this;
    const rangeSeconds = self.parseDuration(self.rangeInput.val());
    for (let i = 0; i < Control.stepValues.length; i++) {
        if (rangeSeconds < self.parseDuration(Control.stepValues[i])) {
            self.rangeInput.val(Control.stepValues[i]);
            break;
        }
    }
    self.evaluate();
};

Control.prototype.decreaseRange = function() {
    const self = this;
    const rangeSeconds = self.parseDuration(self.rangeInput.val());
    for (let i = Control.stepValues.length - 1; i >= 0; i--) {
        if (rangeSeconds > self.parseDuration(Control.stepValues[i])) {
            self.rangeInput.val(Control.stepValues[i]);
            break;
        }
    }
    self.evaluate();
};

Control.prototype.parseDuration = function(rangeText) {
    const rangeRE = new RegExp("^([0-9]+)([ywdhms]+)$");
    const matches = rangeText.match(rangeRE);
    if (!matches || matches.length !== 3) {
        return Control.timeFactors["d"];
    }
    const value = parseInt(matches[1]);
    const unit = matches[2];
    return value * Control.timeFactors[unit];
};

Control.prototype.increaseEnd = function() {
    const self = this;
    const newDate = moment(self.getOrSetEndDate());
    newDate.add(self.parseDuration(self.rangeInput.val()) / 2, 'seconds');
    self.setEndDate(newDate);
    self.evaluate();
};

Control.prototype.decreaseEnd = function() {
    const self = this;
    const newDate = moment(self.getOrSetEndDate());
    newDate.subtract(self.parseDuration(self.rangeInput.val()) / 2, 'seconds');
    self.setEndDate(newDate);
    self.evaluate();
};

Control.prototype.getOrSetEndDate = function() {
    const self = this;
    const date = self.getEndDate();
    self.setEndDate(date);
    return date;
};

Control.prototype.getEndDate = function() {
    const self = this;
    if (!self.endDate || !self.endDate.val()) {
        return moment();
    }
    return self.endDate.data('DateTimePicker').date();
};

Control.prototype.setEndDate = function(date) {
    const self = this;
    self.endDate.data('DateTimePicker').date(date);
};

/**
 * Graph
 */
const Graph = function(element, json, graphsVisible, annotationsVisible) {
    this.el = element;
    this.graphHTML = null;
    this.options = {};
    this.json = json;
    this.graphsVisible = graphsVisible;
    this.annotationsVisible = annotationsVisible;

    this.graphRef = {};
    this.graphRef.data = [];
    this.graphRef.rickshawGraph = null;

    this.alertGraphRef = {};
    this.alertGraphRef.data = [];
    this.alertGraphRef.rickshawGraph = null;

    this.initialize();
};

Graph.numGraphs = 0;

Graph.prototype.initialize = function() {
    const self = this;
    self.id = Graph.numGraphs++;

    // Set default options.
    self.options.id = self.id;

    // Draw graph controls and container from Handlebars template.
    const options = {
        'pathPrefix': PATH_PREFIX,
        'buildVersion': BUILD_VERSION,
        'ruleName': self.json.data.name,
        'activeAlerts': self.json.data.alerts,
        'definition': self.json.data.definition,
    };
    if(self.json.data.alerts) {
        options.activeAlertsLength = self.json.data.alerts.length;
    } else {
        options.activeAlertsLength = 0
    }

    let maxState = 0;
    self.activeAlerts = [];
    for(let i in options.activeAlerts) {
        const activeAt = options.activeAlerts[i].ActiveAt;
        const firedAt = options.activeAlerts[i].FiredAt;
        const resolvedAt = options.activeAlerts[i].ResolvedAt;
        options.activeAlerts[i].id = `activeAlert${self.id}.${i}`;
        options.activeAlerts[i].name = `Alert ${i}`;
        options.activeAlerts[i].Labels = mustacheFormatMap(options.activeAlerts[i].Labels);
        options.activeAlerts[i].Annotations = mustacheFormatMap(options.activeAlerts[i].Annotations);
        options.activeAlerts[i].stateName = alertStateToName[options.activeAlerts[i].State];
        options.activeAlerts[i].stateClass = alertStateToRowClass[options.activeAlerts[i].State];
        options.activeAlerts[i].ActiveAt = mustacheFormatDate(activeAt*1000);
        options.activeAlerts[i].FiredAt = mustacheFormatDate(firedAt*1000);
        options.activeAlerts[i].ResolvedAt = mustacheFormatDate(resolvedAt*1000);
        if (options.activeAlerts[i].State === 2 && options.activeAlerts[i].ResolvedAt === "") {
            options.activeAlerts[i].ResolvedAt = "Never";
        }
        if (options.activeAlerts[i].State > maxState) {
            maxState = options.activeAlerts[i].State;
        }

        const activeAlert = {start: activeAt, value: Mustache.render(popupTemplate, options.activeAlerts[i])};
        if (!isTimestampZero(resolvedAt)) {
            activeAlert.end = resolvedAt;
        } else if (!isTimestampZero(firedAt)) {
            activeAlert.end = self.json.end;
        }
        self.activeAlerts.push(activeAlert);

    }
    options.maxState = alertStateToRowClass[maxState]
    options.graphsDisplay = self.graphsVisible ? "" : 'style="display: none;"';
    options.annotationsDisplay = self.annotationsVisible ? "" : 'style="display: none;"';

    jQuery.extend(options, self.options);
    self.graphHTML = $(Mustache.render(graphTemplate, options));
    self.el.append(self.graphHTML);

    // Get references to all the interesting elements in the graph container and
    // bind event handlers.
    const graphWrapper = self.el.find("#graph_wrapper" + self.id);
    self.queryForm = graphWrapper.find(".query_form");

    self.stackedBtn = self.queryForm.find(".stacked_btn");
    self.stacked = self.queryForm.find("input[name=stacked]");

    self.errorWrapper = self.el.find("#error_wrapper" + self.id).hide();
    self.error = self.errorWrapper.find(".error");
    self.resultWrapper = self.el.find("#result_wrapper" + self.id).hide();
    self.spinner = graphWrapper.find(".spinner");
    self.evalStats = graphWrapper.find(".eval_stats");

    self.graphRef.graphTitle = self.el.find("#expr_graph_title"+self.id);
    self.graphRef.graphArea = graphWrapper.find(".graph_area");
    self.graphRef.graph = self.graphRef.graphArea.find(".graph");
    self.graphRef.yAxis = self.graphRef.graphArea.find(".y_axis");
    self.graphRef.legend = graphWrapper.find(".legend");
    self.graphRef.slider = graphWrapper.find(".slider");
    self.graphRef.timeline = graphWrapper.find(".timeline");

    const alertGraphWrapper = self.el.find("#alert_graph_wrapper" + self.id);
    self.alertGraphRef.graphArea = alertGraphWrapper.find(".graph_area");
    self.alertGraphRef.graph = self.alertGraphRef.graphArea.find(".graph");
    self.alertGraphRef.yAxis = self.alertGraphRef.graphArea.find(".y_axis");
    self.alertGraphRef.legend = alertGraphWrapper.find(".legend");
    self.alertGraphRef.slider = alertGraphWrapper.find(".slider");
    self.alertGraphRef.timeline = alertGraphWrapper.find(".timeline");

    self.isStacked = function() {
        return self.stacked.val() === '1';
    };

    const styleStackBtn = function() {
        const icon = self.stackedBtn.find('.glyphicon');
        if (self.isStacked()) {
            self.stackedBtn.addClass("btn-primary");
            icon.addClass("glyphicon-check");
            icon.removeClass("glyphicon-unchecked");
        } else {
            self.stackedBtn.removeClass("btn-primary");
            icon.addClass("glyphicon-unchecked");
            icon.removeClass("glyphicon-check");
        }
    };
    styleStackBtn();

    self.stackedBtn.click(function() {
        if (self.isStacked() && self.graphRef.json && self.alertGraphRef.json) {
            // If the graph was stacked, the original series data got mutated
            // (scaled) and we need to reconstruct it from the original JSON data.
            self.graphRef.data = self.transformData(self.graphRef.json);
            self.alertGraphRef.data = self.transformData(self.alertGraphRef.json);
        }
        self.stacked.val(self.isStacked() ? '0' : '1');
        styleStackBtn();
        self.updateGraph(self.graphRef);
        self.updateGraph(self.alertGraphRef);
    });

    self.spinner.hide();

    self.initGraphUpdate();
};

Graph.prototype.initGraphUpdate = function() {
    const self = this;
    self.clearError();
    self.params = {
        start: self.json.start * 1000,
        end: self.json.end * 1000,
        step: self.json.step,
    };

    self.handleGraphResponse(self.graphRef, self.json.data.exprQueryResult);
    self.handleGraphResponse(self.alertGraphRef, self.json.data.matrixResult);
};

Graph.prototype.showError = function(msg) {
    const self = this;
    self.error.text(msg);
    self.errorWrapper.show();
    self.resultWrapper.hide();
};

Graph.prototype.clearError = function() {
    const self = this;
    self.error.text('');
    self.errorWrapper.hide();
    self.resultWrapper.show();
};

Graph.prototype.renderLabels = function(labels) {
    const labelStrings = [];
    for (let label in labels) {
        if (label !== "__name__") {
            labelStrings.push("<strong>" + label + "</strong>: " + escapeHTML(labels[label]));
        }
    }
    return "<div class=\"labels\">" + labelStrings.join("<br>") + "</div>";
};

Graph.prototype.metricToTsName = function(labels) {
    let tsName = (labels.__name__ || '') + "{";
    const labelStrings = [];
    for (let label in labels) {
        if (label !== "__name__") {
            labelStrings.push(label + "=\"" + labels[label] + "\"");
        }
    }
    tsName += labelStrings.join(",") + "}";
    return tsName;
};

Graph.prototype.parseValue = function(value) {
    const val = parseFloat(value);
    if (isNaN(val)) {
        // "+Inf", "-Inf", "+Inf" will be parsed into NaN by parseFloat(). The
        // can't be graphed, so show them as gaps (null).
        return null;
    }
    return val;
};

Graph.prototype.transformData = function(json) {
    const self = this;
    const palette = new Rickshaw.Color.Palette();
    if (json.resultType !== "matrix") {
        self.showError("Result is not of matrix type! Please enter a correct expression.");
        return [];
    }
    json.result = json.result || []
    const data = json.result.map(function(ts) {
        let name;
        let labels;
        if (ts.metric === null) {
            name = "scalar";
            labels = {};
        } else {
            name = escapeHTML(self.metricToTsName(ts.metric));
            labels = ts.metric;
        }
        const temp = ts.values.map(function(value) {
            return {
                x: value[0] * 1000,
                y: self.parseValue(value[1])
            };
        });
        return {
            name: name,
            labels: labels,
            data: temp,
            tempData: temp, // Explained in 'updateGraph'.
            color: palette.color()
        };
    });
    data.forEach(function(s) {
        // Insert nulls for all missing steps.
        const newSeries = [];
        let pos = 0;
        for (let t = self.params.start; t <= self.params.end; t += self.params.step) {
            // Allow for floating point inaccuracy.
            let insertNull = true
            while (s.data.length > pos && s.data[pos].x <= t + self.params.step) {
                const pnt = s.data[pos];
                newSeries.push({x: (pnt.x / 1000) | 0, y: pnt.y});
                insertNull = false
                pos++;
            }
            if (insertNull) {
                newSeries.push({x: (t / 1000) | 0, y: null});
            }
        }
        s.data = newSeries;
    });
    return data;
};

Graph.prototype.updateGraph = function(graphRef) {
    const self = this;
    if (graphRef.data.length === 0) { return; }

    // Remove any traces of an existing graph.
    graphRef.legend.empty();
    if (graphRef.graphArea.children().length > 0) {
        graphRef.graph.remove();
        graphRef.yAxis.remove();
    }

    if (graphRef.graphTitle !== undefined) {
        graphRef.graphTitle.html("<u>Graph</u>: '"+ escapeHTML(graphRef.graphJSON.expr)+"'");
    }

    graphRef.graph = $('<div class="graph"></div>');
    graphRef.yAxis = $('<div class="y_axis"></div>');
    graphRef.graphArea.append(graphRef.graph);
    graphRef.graphArea.append(graphRef.yAxis);

    graphRef.data.forEach(function(s) {
        // Padding series with invisible "null" values at the configured x-axis boundaries ensures
        // that graphs are displayed with a fixed x-axis range instead of snapping to the available
        // time range in the data.
        if (s.data[0].x > self.params.start) {
            s.data.unshift({x: (self.params.start / 1000) | 0, y: null});
        }
        if (s.data[s.data.length - 1].x < self.params.end) {
            s.data.push({x: (self.params.end / 1000) | 0, y: null});
        }
    });

    // Now create the new graph.
    graphRef.rickshawGraph = new Rickshaw.Graph({
        element: graphRef.graph[0],
        height: Math.max(graphRef.graph.innerHeight(), 100),
        width: Math.max(graphRef.graph.innerWidth() - 80, 200),
        renderer: (self.isStacked() ? "stack" : "line"),
        interpolation: "linear",
        series: graphRef.data,
        min: "auto",
    });

    // Find and set graph's max/min
    if (self.isStacked() === true) {
        // When stacked is toggled
        let max = 0;
        graphRef.data.forEach(function(timeSeries) {
            let currSeriesMax = 0;
            timeSeries.data.forEach(function(dataPoint) {
                if (dataPoint.y > currSeriesMax && dataPoint.y != null) {
                    currSeriesMax = dataPoint.y;
                }
            });
            max += currSeriesMax;
        });
        graphRef.rickshawGraph.max = max*1.05;
        graphRef.rickshawGraph.min = 0;
    } else {
        let min = Infinity;
        let max = -Infinity;
        graphRef.data.forEach(function(timeSeries) {
            timeSeries.data.forEach(function(dataPoint) {
                if (dataPoint.y < min && dataPoint.y != null) {
                    min = dataPoint.y;
                }
                if (dataPoint.y > max && dataPoint.y != null) {
                    max = dataPoint.y;
                }
            });
        });
        if (min === max) {
            graphRef.rickshawGraph.max = max + 1;
            graphRef.rickshawGraph.min = min - 1;
        } else {
            graphRef.rickshawGraph.max = max + (0.1*(Math.abs(max - min)));
            graphRef.rickshawGraph.min = min - (0.1*(Math.abs(max - min)));
        }
    }

    const xAxis = new Rickshaw.Graph.Axis.Time({ graph: graphRef.rickshawGraph });

    const yAxis = new Rickshaw.Graph.Axis.Y({
        graph: graphRef.rickshawGraph,
        orientation: "left",
        tickFormat: function(y) {
            return self.formatKMBT(y);
        },
        element: graphRef.yAxis[0],
    });

    graphRef.rickshawGraph.render();

    const hoverDetail = new Rickshaw.Graph.HoverDetail({
        graph: graphRef.rickshawGraph,
        formatter: function(series, x, y) {
            const datestr = new Date(x * 1000).toUTCString();
            const date = '<span class="date">' + datestr + '</span>';
            const swatch = '<span class="detail_swatch" style="background-color: ' + series.color + '"></span>';
            const content = swatch + (series.labels.__name__ || 'value') + ": <strong>" + Math.round((y + Number.EPSILON) * 1000) / 1000 + '</strong>';
            return date + '<br>' + content + '<br><strong>Series:</strong><br>' + self.renderLabels(series.labels);
        }
    });

    const legend = new Rickshaw.Graph.Legend({
        graph: graphRef.rickshawGraph,
        element: graphRef.legend[0]
    });

    const highlighter = new Rickshaw.Graph.Behavior.Series.Highlight( {
        graph: graphRef.rickshawGraph,
        legend: legend
    });

    const shelving = new Rickshaw.Graph.Behavior.Series.Toggle({
        graph: graphRef.rickshawGraph,
        legend: legend
    });

    const slider = new Rickshaw.Graph.RangeSlider({
        graph: graphRef.rickshawGraph,
        element: graphRef.slider[0]
    });

    const annotator = new Rickshaw.Graph.Annotate({
        graph: graphRef.rickshawGraph,
        element: graphRef.timeline[0]
    });
    if (self.activeAlerts.length > 0) {
        self.activeAlerts.forEach(function (activeAlert) {
            annotator.add(activeAlert.start, activeAlert.value, activeAlert.end ?? null);
        })
    }
    annotator.update();

    Rickshaw.keys(annotator.data).forEach(function(time) {
        const annotation = annotator.data[time];
        annotation.element.addEventListener("click", function(e) {
            if (annotation !== annotator.active) {
                if (annotator.active) {
                    annotator.active.element.classList.toggle("active");
                    annotator.active.line.classList.toggle("active");
                    annotator.active.boxes.forEach(function(box) {
                        if (box.rangeElement) {
                            box.rangeElement.classList.toggle("active");
                        }
                    });
                }
                annotator.active = annotation;
            } else {
                annotator.active = null;
            }
        }, false);
    });
}

Graph.prototype.resizeGraph = function() {
    const self = this;
    self.resizeGraphInternal(self.graphRef);
    self.resizeGraphInternal(self.alertGraphRef);
};

Graph.prototype.resizeGraphInternal = function(graphRef) {
    if (graphRef.rickshawGraph !== null) {
        graphRef.rickshawGraph.configure({
            width: Math.max(graphRef.graph.innerWidth() - 80, 200),
        });
        graphRef.rickshawGraph.render();
    }
}

Graph.prototype.handleGraphResponse = function(graphRef, json) {
    const self = this;
    // Rickshaw mutates passed series data for stacked graphs, so we need to save
    // the original AJAX response in order to re-transform it into series data
    // when the user disables the stacking.
    graphRef.graphJSON = json;
    graphRef.data = self.transformData(json);
    if (graphRef.data.length === 0) {
        self.showError("No datapoints found.");
        return;
    }
    self.updateGraph(graphRef);
};

Graph.prototype.toFixedUnit = function(value, decimals, unit) {
    const self = this;
    const fixedValue = self.toFixed(value, decimals);
    if (fixedValue === '') {
        return '';
    }
    return fixedValue + unit;
}

Graph.prototype.toFixed = function(value, decimals) {
    const self = this;
    if (value === null) {
        return '';
    }

    if (value === Number.NEGATIVE_INFINITY || value === Number.POSITIVE_INFINITY) {
        return value.toLocaleString();
    }

    if (decimals === null || decimals === undefined) {
        decimals = self.getDecimalsForValue(value);
    }

    const factor = decimals ? Math.pow(10, Math.max(0, decimals)) : 1;
    const formatted = String(Math.round(value * factor) / factor);

    // if exponent return directly
    if (formatted.indexOf('e') !== -1 || value === 0) {
        return formatted;
    }

    const decimalPos = formatted.indexOf('.');
    const precision = decimalPos === -1 ? 0 : formatted.length - decimalPos - 1;
    if (precision < decimals) {
        return (precision ? formatted : formatted + '.') + String(factor).slice(1, decimals - precision + 1);
    }

    return formatted;
}

Graph.prototype.getDecimalsForValue = function(value) {
    const log10 = Math.floor(Math.log(Math.abs(value)) / Math.LN10);
    let dec = -log10 + 1;
    const magn = Math.pow(10, -dec);
    const norm = value / magn; // norm is between 1.0 and 10.0

    // special case for 2.5, requires an extra decimal
    if (norm > 2.25) {
        ++dec;
    }

    if (value % 1 === 0) {
        dec = 0;
    }

    return Math.max(0, dec);
}

Graph.prototype.formatKMBT = function(y) {
    const self = this;
    const decimals = 5;
    var abs_y = Math.abs(y);
    if (abs_y >= 1e24) {
        return self.toFixedUnit(y / 1e24, decimals, "Y");
    } else if (abs_y >= 1e21) {
        return self.toFixedUnit(y / 1e21, decimals, "Z");
    } else if (abs_y >= 1e18) {
        return self.toFixedUnit(y / 1e18, decimals, "E");
    } else if (abs_y >= 1e15) {
        return self.toFixedUnit(y / 1e15, decimals, "P");
    } else if (abs_y >= 1e12) {
        return self.toFixedUnit(y / 1e12, decimals, "T");
    } else if (abs_y >= 1e9) {
        return self.toFixedUnit(y / 1e9, decimals, "G");
    } else if (abs_y >= 1e6) {
        return self.toFixedUnit(y / 1e6, decimals, "M");
    } else if (abs_y >= 1e3) {
        return self.toFixedUnit(y / 1e3, decimals, "k");
    } else if (abs_y >= 1) {
        return self.toFixed(y,decimals);
    } else if (abs_y === 0) {
        return self.toFixed(y, decimals);
    } else if (abs_y <= 1e-24) {
        return self.toFixedUnit(y / 1e-24, decimals, "y");
    } else if (abs_y <= 1e-21) {
        return self.toFixedUnit(y / 1e-21, decimals, "z");
    } else if (abs_y <= 1e-18) {
        return self.toFixedUnit(y / 1e-18, decimals, "a");
    } else if (abs_y <= 1e-15) {
        return self.toFixedUnit(y / 1e-15, decimals, "f");
    } else if (abs_y <= 1e-12) {
        return self.toFixedUnit(y / 1e-12, decimals, "p");
    } else if (abs_y <= 1e-9) {
        return self.toFixedUnit(y / 1e-9, decimals, "n");
    } else if (abs_y <= 1e-6) {
        return self.toFixedUnit(y / 1e-6, decimals, "µ");
    } else if (abs_y <=1e-3) {
        return self.toFixedUnit(y / 1e-3, decimals, "m");
    } else if (abs_y <= 1) {
        return self.toFixed(y, decimals);
    }
}

function greenHtml(text) {
    return '<div style="color:green;">' + text + '</div>';
}

function redHtml(text) {
    return '<div style="color:red;">' + text + '</div>';
}

function initEditor() {
    $("#ruleTextArea").html("# Enter your entire alert rule file here:\n" +
        "# groups:\n" +
        "#   - name: alert.group\n" +
        "#     rules:\n" +
        "#       - alert: Alert Name\n" +
        "#         expr: vector(1)\n" +
        "#         for: 5m\n");
    ace.require("ace/ext/language_tools");
    ace.config.setModuleUrl('ace/mode/yaml_worker', 'https://cdnjs.cloudflare.com/ajax/libs/ace/1.8.1/worker-yaml.min.js')
    const e = ace.edit("ruleTextArea");
    e.session.setMode("ace/mode/yaml");
    e.session.setUseWrapMode(true);
    e.setTheme("ace/theme/xcode");
    e.setFontSize("10pt");
    e.setOptions({
        useWorker: true,
        enableAutoIndent: true,
        enableLiveAutocompletion: true
    });
    e.focus();
}

function init() {
    moment.relativeTimeThreshold('ss', 60);
    moment.updateLocale('en', {
        relativeTime : {
            s: function (number, withoutSuffix, key, isFuture){
                return number + ' seconds';
            }
        }
    });

    $.ajaxSetup({
        cache: false
    });

    $("#showAll").click(function() {
        $(".alert_details").show();
    });

    $("#hideAll").click(function() {
        $(".alert_details").hide();
    });


    $.ajax({
        url: PATH_PREFIX + "/static/js/alert_testing/popup_template.handlebar?v=" + BUILD_VERSION,
        success: function(data) {
            popupTemplate = data;
            Mustache.parse(data);
            $.ajax({
                url: PATH_PREFIX + "/static/js/alert_testing/graph_template.handlebar?v=" + BUILD_VERSION,
                success: function(data) {
                    graphTemplate = data;
                    Mustache.parse(data);
                    const control = new Control(
                        {
                            end_input: endDate,
                        }
                    );
                    initEditor();
                }
            });
        }
    });
}

$(init);
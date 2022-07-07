package block

import (
	"time"
)

type PlanEntry[V any] interface {
	Start() int64
	End() int64
	Step() int64
	Data() *V
}

func NewPlanEntry[V any](start int64, end int64, step int64, data *V) PlanEntry[V] {
	return &planEntry[V]{
		start: start,
		end:   end,
		step:  step,
		data:  data,
	}
}

type planEntry[V any] struct {
	start int64
	end   int64
	step  int64
	data  *V
}

func (p *planEntry[V]) Start() int64 {
	return p.start
}

func (p *planEntry[V]) End() int64 {
	return p.end
}

func (p *planEntry[V]) Step() int64 {
	return p.step
}

func (p *planEntry[V]) Data() *V {
	return p.data
}

type Planner[V any] interface {
	Plan(transform func(int64, int64, int64) []PlanEntry[V]) [][]PlanEntry[V]
}

func NewPlanner[V any](startTime time.Time, endTime time.Time, blockDuration int64, sampleInterval time.Duration) Planner[V] {
	return &planner[V]{
		startTime:      startTime,
		endTime:        endTime,
		blockDuration:  blockDuration,
		sampleInterval: sampleInterval,
	}
}

type planner[V any] struct {
	startTime      time.Time
	endTime        time.Time
	blockDuration  int64
	sampleInterval time.Duration
}

func (p *planner[V]) Plan(transform func(int64, int64, int64) []PlanEntry[V]) [][]PlanEntry[V] {
	var results [][]PlanEntry[V]
	startInMs := p.startTime.Unix() * int64(time.Second/time.Millisecond)
	endInMs := p.endTime.Unix() * int64(time.Second/time.Millisecond)
	blockStart := p.blockDuration * (startInMs / p.blockDuration)
	stepDuration := int64(p.sampleInterval / (time.Millisecond / time.Nanosecond))
	for ; blockStart <= endInMs; blockStart = blockStart + p.blockDuration {
		blockEnd := blockStart + p.blockDuration - 1
		currStart := p.max(blockStart/int64(time.Second/time.Millisecond), p.startTime.Unix())
		startWithAlignment := p.evalTimestamp(time.Unix(currStart, 0).UTC().UnixNano(), stepDuration)
		for startWithAlignment.Unix() < currStart {
			startWithAlignment = startWithAlignment.Add(p.sampleInterval)
		}
		end := time.Unix(p.min(blockEnd/int64(time.Second/time.Millisecond), p.endTime.Unix()), 0).UTC()
		if end.Equal(startWithAlignment) || end.Before(startWithAlignment) {
			break
		}

		blockPlan := p.planBlock(startWithAlignment, end, stepDuration, transform)
		results = append(results, blockPlan)
	}
	return results
}

func (p *planner[V]) planBlock(blockStart time.Time, blockEnd time.Time, stepDuration int64, transform func(int64, int64, int64) []PlanEntry[V]) []PlanEntry[V] {
	var plan []PlanEntry[V]
	start := blockStart.UnixNano() / int64(time.Millisecond/time.Nanosecond)
	end := blockEnd.UnixNano() / int64(time.Millisecond/time.Nanosecond)
	chunkDuration := (end - start) / 4

	chunkStart := start
	for ; chunkStart <= end; chunkStart = chunkStart + chunkDuration {
		chunkEnd := chunkStart + chunkDuration - 1
		if chunkEnd > end {
			break
		}
		for _, pe := range transform(chunkStart, chunkEnd, stepDuration) {
			plan = append(plan, pe)
		}
	}
	return plan
}

func (p *planner[V]) max(x, y int64) int64 {
	if x > y {
		return x
	}
	return y
}

func (p *planner[V]) min(x, y int64) int64 {
	if x < y {
		return x
	}
	return y
}

func (p *planner[V]) evalTimestamp(startTime int64, stepDuration int64) time.Time {
	var (
		offset = stepDuration
		adjNow = startTime - stepDuration
		base   = adjNow - (adjNow % stepDuration)
	)

	return time.Unix(0, base+offset).UTC()
}

package downsample

import (
	"github.com/prometheus/prometheus/promql"
	"math"
)

type Downsampler interface {
	Downsample(points []promql.Point, targetPointCount int) []promql.Point
}

type point struct {
	X float64
	Y float64
}

func calculateAverageDataPoint(points []promql.Point) (avg point) {
	for _, p := range points {
		avg.X += float64(p.T)
		avg.Y += p.V
	}
	l := float64(len(points))
	avg.X /= l
	avg.Y /= l
	return avg
}

func calculateTriangleArea(pa promql.Point, pb point, pc promql.Point) float64 {
	i := pa.T - pc.T
	j := pb.Y - pa.V
	k := float64(i) * j
	l := float64(pa.T) - pb.X
	m := pc.V - pa.V
	n := l * m
	area := (k - n) * 0.5
	return math.Abs(area)
}

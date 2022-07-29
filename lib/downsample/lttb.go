package downsample

import (
	"github.com/prometheus/prometheus/promql"
	"math"
)

func NewLttbDownsampler() Downsampler {
	return &lttb{}
}

type lttb struct {
}

func (d *lttb) Downsample(points []promql.Point, targetPointCount int) []promql.Point {
	if targetPointCount >= len(points) || targetPointCount == 0 {
		return points
	}

	bucketSize := float64(len(points)-2) / float64(targetPointCount-2)
	sourcePointCount := len(points)

	sampledData := make([]point, 0, targetPointCount)
	sampledData = append(sampledData, point{X: float64(points[0].T), Y: points[0].V})

	bucketLow := 1
	bucketMiddle := int(math.Floor(bucketSize)) + 1

	var prevMaxAreaPoint int
	for i := 0; i < targetPointCount-2; i++ {
		bucketHigh := int(math.Floor(float64(i+2)*bucketSize)) + 1
		if bucketHigh >= sourcePointCount-1 {
			bucketHigh = sourcePointCount - 2
		}

		avgPoint := calculateAverageDataPoint(points[bucketMiddle : bucketHigh+1])

		currBucketStart := bucketLow
		currBucketEnd := bucketMiddle
		pointA := points[prevMaxAreaPoint]

		maxArea := -1.0
		var maxAreaPoint int
		for ; currBucketStart < currBucketEnd; currBucketStart++ {
			area := calculateTriangleArea(pointA, avgPoint, points[currBucketStart])
			if area > maxArea {
				maxArea = area
				maxAreaPoint = currBucketStart
			}
		}

		p := points[maxAreaPoint]
		sampledData = append(sampledData, point{X: float64(p.T), Y: p.V})
		prevMaxAreaPoint = maxAreaPoint

		bucketLow = bucketMiddle
		bucketMiddle = bucketHigh
	}

	p := points[sourcePointCount-1]
	sampledData = append(sampledData, point{X: float64(p.T), Y: p.V})

	result := make([]promql.Point, len(sampledData))
	for i, s := range sampledData {
		result[i] = promql.Point{
			T: int64(s.X),
			V: s.Y,
		}
	}

	return result
}

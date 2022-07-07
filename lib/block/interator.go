package block

import (
	"fmt"
	"github.com/kadaan/promutil/lib/encoding"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
)

type Iterator interface {
	Next() (more bool, err error, value *promql.Sample)
}

func (b *block) NewIterator() Iterator {
	blockLength := len(b.Labels)
	if blockLength == 0 {
		return &emptyBlockIterator{}
	}
	blockPosition := 0
	timestampSampleLength := -1
	timestampSamplePosition := 0
	valueSampleLength := -1
	valueSamplePosition := 0
	for {
		if blockPosition >= blockLength {
			return &emptyBlockIterator{}
		}
		if timestampSampleLength < 0 {
			timestampSampleLength = len(b.Timestamps[blockPosition])
		}
		if valueSampleLength < 0 {
			valueSampleLength = len(b.Values[blockPosition])
		}
		if timestampSamplePosition < timestampSampleLength && valueSamplePosition < valueSampleLength {
			break
		}
		blockPosition++
		timestampSampleLength = -1
		valueSampleLength = -1
	}

	return &blockIterator{
		blockPosition:           blockPosition,
		blockLength:             blockLength,
		labelDecoder:            encoding.NewDoubleDeltaDecoder[int64](),
		timestampSamplePosition: timestampSamplePosition,
		timestampSampleLength:   timestampSampleLength,
		timestampDecoder:        encoding.NewDoubleDeltaDecoder[int64](),
		timestampIterator:       nil,
		valueSamplePosition:     valueSamplePosition,
		valueSampleLength:       valueSampleLength,
		valueDecoder:            encoding.NewDoubleDeltaDecoder[float64](),
		valueIterator:           nil,
		next:                    nil,
		block:                   b,
	}
}

type blockIterator struct {
	blockPosition           int
	blockLength             int
	labelDecoder            encoding.Decoder[int64]
	timestampSampleLength   int
	timestampSamplePosition int
	timestampDecoder        encoding.Decoder[int64]
	timestampIterator       encoding.ValueIterator[int64]
	valueSampleLength       int
	valueSamplePosition     int
	valueDecoder            encoding.Decoder[float64]
	valueIterator           encoding.ValueIterator[float64]
	next                    *promql.Sample
	block                   *block
}

type emptyBlockIterator struct {
}

func (b *emptyBlockIterator) Next() (more bool, err error, next *promql.Sample) {
	return false, nil, nil
}

func (b *blockIterator) Next() (more bool, err error, next *promql.Sample) {
	if b.next == nil {
		stringsPos := 0
		var strings []string
		for _, v := range b.block.Labels[b.blockPosition] {
			labelIterator := b.labelDecoder.Decode(v)
			hasMore, err, val := labelIterator.Next()
			if err != nil {
				return false, err, nil
			}
			strings = append(strings, b.block.Strings[val])
			stringsPos++
			for hasMore {
				hasMore, err, val = labelIterator.Next()
				if err != nil {
					return false, err, nil
				}
				strings = append(strings, b.block.Strings[val])
				stringsPos++
			}
		}
		b.next = &promql.Sample{
			Metric: labels.FromStrings(strings...),
		}
	}

	if b.timestampIterator == nil {
		b.timestampIterator = b.timestampDecoder.Decode(b.block.Timestamps[b.blockPosition][b.timestampSamplePosition])
	}
	moreTimestamps, err, timestamp := b.timestampIterator.Next()
	if err != nil {
		return false, err, nil
	}
	if !moreTimestamps {
		b.timestampIterator = nil
	}
	b.next.T = timestamp

	if b.valueIterator == nil {
		b.valueIterator = b.valueDecoder.Decode(b.block.Values[b.blockPosition][b.valueSamplePosition])
	}
	moreValues, err, val := b.valueIterator.Next()
	if err != nil {
		return false, err, nil
	}
	if !moreValues {
		b.valueIterator = nil
	}
	b.next.V = val

	next = b.next
	more = false
	for {
		if !moreTimestamps {
			if b.timestampSampleLength < 0 {
				b.timestampSampleLength = len(b.block.Timestamps[b.blockPosition])
				b.timestampSamplePosition = 0
			} else {
				b.timestampSamplePosition++
			}
			if b.timestampSamplePosition < b.timestampSampleLength {
				moreTimestamps = true
			}
		}
		if !moreValues {
			if b.valueSampleLength < 0 {
				b.valueSampleLength = len(b.block.Values[b.blockPosition])
				b.valueSamplePosition = 0
			} else {
				b.valueSamplePosition++
			}
			if b.valueSamplePosition < b.valueSampleLength {
				moreValues = true
			}
		}
		if moreTimestamps != moreValues {
			return false, fmt.Errorf("timestamp/value count mismatch"), nil
		}
		if moreTimestamps && moreValues {
			more = true
			break
		}
		b.blockPosition++
		if b.blockPosition >= b.blockLength {
			break
		}
		b.timestampSampleLength = -1
		b.valueSampleLength = -1
		b.labelDecoder.Reset()
		b.timestampDecoder.Reset()
		b.valueDecoder.Reset()
		b.next = nil
	}
	return
}

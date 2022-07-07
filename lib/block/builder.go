package block

import (
	"github.com/kadaan/promutil/lib/encoding"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/prompb"
)

type BlockBuilder interface {
	Add(timeSeries []*prompb.TimeSeries) error
	GetBlock() Block
}

func NewBlockBuilder(start int64, end int64, step int64) BlockBuilder {
	return &blockBuilder{
		dictionary: encoding.NewDictionary(),
		start:      start,
		end:        end,
		step:       step,
		labels:     [][]encoding.EncodedValue[int64]{},
		timestamps: [][]encoding.EncodedValue[int64]{},
		values:     [][]encoding.EncodedValue[float64]{},
	}
}

type blockBuilder struct {
	dictionary encoding.Dictionary
	start      int64
	end        int64
	step       int64
	labels     [][]encoding.EncodedValue[int64]
	timestamps [][]encoding.EncodedValue[int64]
	values     [][]encoding.EncodedValue[float64]
}

func (b *blockBuilder) Add(timeSeries []*prompb.TimeSeries) error {
	for _, ts := range timeSeries {
		started := false
		labelDictionary := b.dictionary.GetView()
		timestampEncoder := encoding.NewDoubleDeltaEncoder[int64]()
		valueEncoder := encoding.NewDoubleDeltaEncoder[float64]()
		for _, s := range ts.Samples {
			if value.IsStaleNaN(s.Value) {
				continue
			} else if !started {
				started = true
				for _, l := range ts.Labels {
					err := labelDictionary.Add(l.Name)
					if err != nil {
						return err
					}
					err = labelDictionary.Add(l.Value)
					if err != nil {
						return err
					}
				}
			}
			timestamp := b.round(float64(s.Timestamp), float64(b.step))
			err := timestampEncoder.Add(timestamp)
			if err != nil {
				return err
			}
			err = valueEncoder.Add(s.Value)
			if err != nil {
				return err
			}
		}
		labelKeys := labelDictionary.Finish()
		timestamps := timestampEncoder.Finish()
		values := valueEncoder.Finish()
		if len(labelKeys) > 0 && len(timestamps) > 0 && len(values) > 0 {
			b.labels = append(b.labels, labelKeys)
			b.timestamps = append(b.timestamps, timestamps)
			b.values = append(b.values, values)
		}
	}
	return nil
}

func (b *blockBuilder) GetBlock() Block {
	return &block{
		Start:      b.start,
		End:        b.end,
		Step:       b.step,
		Strings:    b.dictionary.GetValues(),
		Labels:     b.labels,
		Timestamps: b.timestamps,
		Values:     b.values,
	}
}

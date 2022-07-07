package block

import (
	"github.com/kadaan/promutil/lib/encoding"
	"math"
)

type Block interface {
	NewIterator() Iterator
}

func NewEmptyBlock() Block {
	return &block{
		Start:      0,
		End:        0,
		Step:       0,
		Strings:    []string{},
		Labels:     [][]encoding.EncodedValue[int64]{},
		Timestamps: [][]encoding.EncodedValue[int64]{},
		Values:     [][]encoding.EncodedValue[float64]{},
	}
}

func (b *blockBuilder) round(x, unit float64) int64 {
	return int64(math.Round(x/unit) * unit)
}

type block struct {
	Start      int64                              `json:"start"`
	End        int64                              `json:"end"`
	Step       int64                              `json:"step"`
	Strings    []string                           `json:"strings"`
	Labels     [][]encoding.EncodedValue[int64]   `json:"labels"`
	Timestamps [][]encoding.EncodedValue[int64]   `json:"timestamps"`
	Values     [][]encoding.EncodedValue[float64] `json:"values"`
}

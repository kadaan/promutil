package encoding

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type kv struct {
	Key   string
	Value int64
}

type Dictionary interface {
	GetView() DictionaryView
	GetValues() []string
}

type dictionary struct {
	stringCount int64
	stringMap   map[string]int64
}

func NewDictionary() Dictionary {
	return &dictionary{
		stringCount: 0,
		stringMap:   map[string]int64{},
	}
}

func (d *dictionary) GetView() DictionaryView {
	return &dictionaryView{
		closed:     false,
		dictionary: d,
		values:     NewDoubleDeltaEncoder[int64](),
	}
}

func (d *dictionary) GetValues() []string {
	var dictionaryPairs []kv
	for k, v := range d.stringMap {
		dictionaryPairs = append(dictionaryPairs, kv{k, v})
	}

	sort.Slice(dictionaryPairs, func(i, j int) bool {
		return dictionaryPairs[i].Value < dictionaryPairs[j].Value
	})

	var values = make([]string, len(dictionaryPairs))
	for i, kv := range dictionaryPairs {
		values[i] = kv.Key
	}

	return values
}

type DictionaryView interface {
	Add(value string) error
	Finish() []EncodedValue[int64]
}

type dictionaryView struct {
	closed     bool
	dictionary *dictionary
	values     Encoder[int64]
}

func (d *dictionaryView) Add(value string) error {
	if d.closed {
		return fmt.Errorf("encoder is closed")
	}
	var ok bool
	var id int64
	if id, ok = d.dictionary.stringMap[value]; !ok {
		id = d.dictionary.stringCount
		(d.dictionary.stringMap)[value] = id
		d.dictionary.stringCount++
	}
	err := d.values.Add(id)
	if err != nil {
		return err
	}
	return nil
}

func (d *dictionaryView) Finish() []EncodedValue[int64] {
	d.closed = true
	return d.values.Finish()
}

type Number interface {
	int64 | float64
}

type EncodedValue[V Number] struct {
	Count int
	Value V
}

func (f EncodedValue[V]) MarshalJSON() ([]byte, error) {
	if f.Count == 1 {
		return json.Marshal(f.Value)
	}
	var val V
	switch any(&val).(type) {
	case *int64:
		return json.Marshal(fmt.Sprintf("%d:%d", f.Count, f.Value))
	case *float64:
		return json.Marshal(fmt.Sprintf("%d:%s", f.Count,
			strconv.FormatFloat(any(f.Value).(float64), 'f', -1, 64)))
	}
	return nil, fmt.Errorf("unsupported value type")
}

func (f *EncodedValue[V]) UnmarshalJSON(b []byte) error {
	var val V
	switch any(&val).(type) {
	case *int64:
		var num int64
		if err := json.Unmarshal(b, &num); err == nil {
			f.Count = 1
			f.Value = any(num).(V)
			return nil
		}
	case *float64:
		var num float64
		if err := json.Unmarshal(b, &num); err == nil {
			f.Count = 1
			f.Value = any(num).(V)
			return nil
		}
	}
	var str string
	if err := json.Unmarshal(b, &str); err != nil {
		return err
	}
	parts := strings.Split(str, ":")
	c, err := strconv.Atoi(parts[0])
	if err != nil {
		return err
	}
	f.Count = c
	switch any(&val).(type) {
	case *int64:
		v, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return err
		}
		f.Value = any(v).(V)
	case *float64:
		v, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return err
		}
		f.Value = any(v).(V)
	}
	return nil
}

type Encoder[V Number] interface {
	Add(value V) error
	Finish() []EncodedValue[V]
}

type encoder[V Number] struct {
	closed       bool
	mode         int
	count        int
	value        V
	delta        V
	deltaOfDelta V
	values       []EncodedValue[V]
}

func NewDoubleDeltaEncoder[V Number]() Encoder[V] {
	return &encoder[V]{
		closed:       false,
		mode:         0,
		count:        0,
		value:        0,
		delta:        0,
		deltaOfDelta: 0,
		values:       []EncodedValue[V]{},
	}
}

func (e *encoder[V]) Add(value V) error {
	if e.closed {
		return fmt.Errorf("encoder is closed")
	}
	if e.mode == 0 {
		e.value = value
		e.values = append(e.values, EncodedValue[V]{1, value})
		e.mode = 1
		return nil
	}

	delta := value - e.value
	e.value = value
	if e.mode == 1 {
		e.delta = delta
		e.values = append(e.values, EncodedValue[V]{1, delta})
		e.mode = 2
		return nil
	}

	deltaOfDelta := delta - e.delta
	e.delta = delta
	if e.mode == 2 {
		e.deltaOfDelta = deltaOfDelta
		e.count++
		e.mode = 3
		return nil
	}

	if deltaOfDelta == e.deltaOfDelta {
		e.count++
		return nil
	}

	e.values = append(e.values, EncodedValue[V]{e.count, e.deltaOfDelta})
	e.deltaOfDelta = deltaOfDelta
	e.count = 1
	return nil
}

func (e *encoder[V]) Finish() []EncodedValue[V] {
	e.closed = true
	if e.mode == 3 {
		e.values = append(e.values, EncodedValue[V]{e.count, e.deltaOfDelta})
	}
	return e.values
}

type Decoder[V Number] interface {
	Decode(value EncodedValue[V]) ValueIterator[V]
	Reset()
}

type decoder[V Number] struct {
	mode  int
	count int
	value V
	delta V
}

func NewDoubleDeltaDecoder[V Number]() Decoder[V] {
	return &decoder[V]{
		mode:  0,
		value: 0,
		delta: 0,
	}
}

func (d *decoder[V]) Decode(value EncodedValue[V]) ValueIterator[V] {
	return &valueIterator[V]{
		decoder: d,
		value:   value,
		count:   0,
	}
}

func (d *decoder[V]) Reset() {
	d.mode = 0
	d.value = 0
	d.delta = 0
}

type ValueIterator[V Number] interface {
	Next() (bool, error, V)
}

type valueIterator[V Number] struct {
	decoder  *decoder[V]
	value    EncodedValue[V]
	count    int
	repCount int
	repValue V
}

func (s *valueIterator[V]) Next() (more bool, err error, value V) {
	if s.repCount == 0 {
		if s.decoder.mode == 0 {
			s.decoder.value = s.value.Value
			s.decoder.mode = 1
			return false, nil, s.decoder.value
		}
		if s.decoder.mode == 1 {
			s.decoder.delta = s.value.Value
			s.decoder.value = s.decoder.value + s.decoder.delta
			s.decoder.mode = 2
			return false, nil, s.decoder.value
		}
		if s.value.Count == 1 {
			s.decoder.delta = s.decoder.delta + s.value.Value
			s.decoder.value = s.decoder.value + s.decoder.delta
			return false, nil, s.decoder.value
		}
	}
	hasMore := s.count < s.value.Count-1
	s.decoder.delta = s.decoder.delta + s.value.Value
	s.decoder.value = s.decoder.value + s.decoder.delta
	s.count++
	return hasMore, nil, s.decoder.value
}

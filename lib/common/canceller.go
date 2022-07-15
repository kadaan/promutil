package common

import "sync"

type Canceller interface {
	Cancel()
}

type canceller struct {
	C    chan struct{}
	once sync.Once
}

func NewCanceller() *canceller {
	return &canceller{
		C: make(chan struct{}),
	}
}

func (s *canceller) Cancel() {
	s.once.Do(func() {
		close(s.C)
	})
}

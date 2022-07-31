package common

import "sync"

type Canceller interface {
	C() chan struct{}
	Cancelled() bool
	Cancel()
}

type canceller struct {
	cancelled bool
	c         chan struct{}
	once      sync.Once
}

func NewCanceller() *canceller {
	return &canceller{
		cancelled: false,
		c:         make(chan struct{}),
	}
}

func (s *canceller) C() chan struct{} {
	return s.c
}

func (s *canceller) Cancelled() bool {
	return s.cancelled
}

func (s *canceller) Cancel() {
	s.once.Do(func() {
		s.cancelled = true
		close(s.c)
	})
}

package server

import "sync/atomic"

// Readiness records the decision supplied by the composition root. It starts
// false; process startup alone does not establish business dependency readiness.
type Readiness struct {
	ready atomic.Bool
}

func NewReadiness() *Readiness {
	return &Readiness{}
}

func (r *Readiness) Set(ready bool) {
	r.ready.Store(ready)
}

func (r *Readiness) Ready() bool {
	return r.ready.Load()
}

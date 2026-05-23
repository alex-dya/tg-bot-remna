package handlers

import (
	"sync"
	"time"
)

type adminState string

const (
	stateAwaitList adminState = "await_list"
)

type stateEntry struct {
	state   adminState
	setAt   time.Time
}

type stateStore struct {
	mu sync.Mutex
	m  map[int64]stateEntry
}

func newStateStore() *stateStore {
	return &stateStore{m: make(map[int64]stateEntry)}
}

const stateTTL = 5 * time.Minute

func (s *stateStore) set(uid int64, st adminState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[uid] = stateEntry{state: st, setAt: time.Now()}
}

func (s *stateStore) get(uid int64) (adminState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[uid]
	if !ok {
		return "", false
	}
	if time.Since(e.setAt) > stateTTL {
		delete(s.m, uid)
		return "", false
	}
	return e.state, true
}

func (s *stateStore) clear(uid int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, uid)
}

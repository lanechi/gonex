package scheduler

import "sync"

type overlapGate struct {
	mu     sync.Mutex
	policy OverlapPolicy
	active int
	queued bool
}

func (gate *overlapGate) run(handler func()) (executed, queued bool) {
	gate.mu.Lock()
	if gate.policy == AllowOverlap {
		gate.active++
		gate.mu.Unlock()
		defer func() { gate.mu.Lock(); gate.active--; gate.mu.Unlock() }()
		handler()
		return true, false
	}
	if gate.active > 0 {
		if gate.policy == QueueOne && !gate.queued {
			gate.queued = true
			gate.mu.Unlock()
			return false, true
		}
		gate.mu.Unlock()
		return false, false
	}
	gate.active = 1
	gate.mu.Unlock()
	for {
		handler()
		gate.mu.Lock()
		if gate.policy == QueueOne && gate.queued {
			gate.queued = false
			gate.mu.Unlock()
			continue
		}
		gate.active = 0
		gate.mu.Unlock()
		return true, false
	}
}

func (gate *overlapGate) isRunning() bool {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	return gate.active > 0
}

package scheduler

import "sync"

type overlapGate struct {
	mu            sync.Mutex
	policy        OverlapPolicy
	active        int
	queuedHandler func()
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
		if gate.policy == QueueOne && gate.queuedHandler == nil {
			gate.queuedHandler = handler
			gate.mu.Unlock()
			return false, true
		}
		gate.mu.Unlock()
		return false, false
	}
	gate.active = 1
	current := handler
	gate.mu.Unlock()
	for {
		current()
		gate.mu.Lock()
		if gate.policy == QueueOne && gate.queuedHandler != nil {
			current = gate.queuedHandler
			gate.queuedHandler = nil
			gate.mu.Unlock()
			continue
		}
		gate.active = 0
		gate.mu.Unlock()
		return true, false
	}
}

func (gate *overlapGate) isRunning() bool {
	if gate == nil {
		return false
	}
	gate.mu.Lock()
	defer gate.mu.Unlock()
	return gate.active > 0
}

func (gate *overlapGate) setPolicy(policy OverlapPolicy) {
	if gate == nil {
		return
	}
	gate.mu.Lock()
	gate.policy = policy
	if policy != QueueOne {
		gate.queuedHandler = nil
	}
	gate.mu.Unlock()
}

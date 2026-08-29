package scheduler

import "sync"

type overlapGate struct {
	mu            sync.Mutex
	policy        OverlapPolicy
	active        int
	queued        bool
	queuedHandler func()
}

func (gate *overlapGate) run(handler func()) (executed, queued bool) {
	return gate.runWithToken(gate, handler)
}

func (gate *overlapGate) runWithToken(_ any, handler func()) (executed, queued bool) {
	if gate == nil || handler == nil {
		return false, false
	}
	gate.mu.Lock()
	if gate.policy == AllowOverlap {
		gate.active++
		gate.mu.Unlock()
		gate.runAndDrain(handler)
		return true, false
	}
	if gate.active > 0 {
		if gate.policy == QueueOne && !gate.queued {
			gate.queued = true
			gate.queuedHandler = handler
			gate.mu.Unlock()
			return false, true
		}
		gate.mu.Unlock()
		return false, false
	}
	gate.active = 1
	gate.mu.Unlock()
	gate.runAndDrain(handler)
	return true, false
}

// runAndDrain finishes one accepted execution and, when it is the last active
// execution, drains one trigger that was already accepted by QueueOne. The
// queued trigger remains accepted even if Replace changes the policy while the
// current generation is still running; policy changes affect future triggers,
// not work that was already queued.
func (gate *overlapGate) runAndDrain(handler func()) {
	current := handler
	for current != nil {
		current()
		gate.mu.Lock()
		if gate.active > 0 {
			gate.active--
		}
		if gate.active == 0 && gate.queued {
			current = gate.queuedHandler
			gate.queued = false
			gate.queuedHandler = nil
			gate.active = 1
			gate.mu.Unlock()
			continue
		}
		gate.mu.Unlock()
		return
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
	gate.mu.Unlock()
}

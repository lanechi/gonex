package scheduler

import "sync"

type overlapGate struct {
	mu            sync.Mutex
	policy        OverlapPolicy
	active        int
	activeToken   any
	queued        bool
	queuedToken   any
	queuedHandler func()
}

func (gate *overlapGate) run(handler func()) (executed, queued bool) {
	return gate.runWithToken(gate, handler)
}

func (gate *overlapGate) runWithToken(token any, handler func()) (executed, queued bool) {
	if gate == nil || handler == nil {
		return false, false
	}
	gate.mu.Lock()
	if gate.policy == AllowOverlap {
		if gate.active == 0 {
			gate.activeToken = token
		}
		gate.active++
		gate.mu.Unlock()
		gate.executeAccepted(token, handler)
		return true, false
	}
	if gate.active > 0 {
		if gate.policy == QueueOne && !gate.queued {
			gate.queued = true
			gate.queuedToken = token
			// Repeated triggers from the current generation replay the active
			// generation's handler. A trigger from a replacement generation must
			// retain that replacement handler until the active generation drains.
			if token != gate.activeToken {
				gate.queuedHandler = handler
			}
			gate.mu.Unlock()
			return false, true
		}
		gate.mu.Unlock()
		return false, false
	}
	gate.active = 1
	gate.activeToken = token
	gate.mu.Unlock()
	gate.executeAccepted(token, handler)
	return true, false
}

// executeAccepted owns completion for an execution that was already admitted.
// Only the execution that observes active reaching zero may take ownership of a
// previously accepted QueueOne trigger. This keeps the caller that merely
// queued work non-blocking while also allowing AllowOverlap -> QueueOne policy
// transitions to drain the queue after the last overlapping execution exits.
func (gate *overlapGate) executeAccepted(token any, handler func()) {
	currentToken := token
	current := handler
	for current != nil {
		current()

		gate.mu.Lock()
		if gate.active > 0 {
			gate.active--
		}
		if gate.active != 0 || !gate.queued {
			if gate.active == 0 {
				gate.activeToken = nil
			}
			gate.mu.Unlock()
			return
		}

		nextToken := gate.queuedToken
		next := gate.queuedHandler
		if next == nil && nextToken == currentToken {
			next = current
		}
		gate.queued = false
		gate.queuedToken = nil
		gate.queuedHandler = nil
		if next == nil {
			gate.activeToken = nil
			gate.mu.Unlock()
			return
		}
		gate.active = 1
		gate.activeToken = nextToken
		currentToken = nextToken
		current = next
		gate.mu.Unlock()
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
	// A trigger accepted under QueueOne is committed work. Policy changes only
	// affect future triggers; clearing the accepted queue here would silently
	// lose work during Replace.
	gate.mu.Unlock()
}

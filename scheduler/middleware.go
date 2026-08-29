package scheduler

import "github.com/lanechi/gonex/logging"

func (manager *manager) middlewareSnapshot(jobMiddleware []Middleware) []Middleware {
	manager.mu.RLock()
	middleware := append([]Middleware(nil), manager.middleware...)
	manager.mu.RUnlock()
	return append(middleware, jobMiddleware...)
}

func (manager *manager) currentLogger() logging.Logger {
	manager.mu.RLock()
	logger := manager.logger
	manager.mu.RUnlock()
	if logger == nil {
		return logging.NewNopLogger()
	}
	return logger
}

package scheduler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/lanechi/gonex/logging"
)

// Option configures a scheduler at construction time.
type Option func(*managerOptions) error
type managerOptions struct {
	location  *time.Location
	logger    logging.Logger
	loggerSet bool
}

// WithLocation sets the default location used for cron expressions.
func WithLocation(location *time.Location) Option {
	return func(options *managerOptions) error {
		if location == nil {
			return errors.New("scheduler location must not be nil")
		}
		options.location = location
		return nil
	}
}

// WithLogger sends scheduler lifecycle and job outcome logs through logger.
func WithLogger(logger logging.Logger) Option {
	return func(options *managerOptions) error {
		if logger == nil {
			return errors.New("scheduler logger must not be nil")
		}
		options.logger = logger
		options.loggerSet = true
		return nil
	}
}

// New creates an independent scheduler.
func New(options ...Option) (Scheduler, error) {
	configuration := managerOptions{location: time.Local, logger: logging.NewNopLogger()}
	for _, option := range options {
		if option != nil {
			if err := option(&configuration); err != nil {
				return nil, err
			}
		}
	}
	return &manager{jobs: make(map[string]*jobRecord), location: configuration.location, logger: configuration.logger, loggerConfigured: configuration.loggerSet, context: context.Background()}, nil
}

// MustNew creates a scheduler and panics when an Option is invalid.
func MustNew(options ...Option) Scheduler {
	manager, err := New(options...)
	if err != nil {
		panic(fmt.Sprintf("create scheduler: %v", err))
	}
	return manager
}

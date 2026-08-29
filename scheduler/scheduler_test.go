package scheduler

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lanechi/gonex/logging"
)

func TestManagerRunsCronEveryAndOnce(t *testing.T) {
	manager, err := New()
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan string, 3)
	if err := manager.Add(Job{
		Name:           "cron",
		Schedule:       Cron{Expr: "*/1 * * * * *"},
		RunImmediately: true,
		Handler: func(context.Context) error {
			results <- "cron"
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Add(Job{
		Name:           "every",
		Schedule:       Every{Duration: time.Hour},
		RunImmediately: true,
		Handler: func(context.Context) error {
			results <- "every"
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Add(Job{
		Name:     "once",
		Schedule: Once{At: time.Now().Add(50 * time.Millisecond)},
		Handler: func(context.Context) error {
			results <- "once"
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		manager.Stop()
		if err := manager.Wait(context.Background()); err != nil {
			t.Error(err)
		}
	}()

	seen := make(map[string]bool)
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for len(seen) < 3 {
		select {
		case result := <-results:
			seen[result] = true
		case <-deadline.C:
			t.Fatalf("jobs completed = %#v, want cron, every, once", seen)
		}
	}
	jobs := manager.Jobs()
	if len(jobs) != 3 {
		t.Fatalf("Jobs length = %d, want 3", len(jobs))
	}
}

func TestManagerValidatesNamesAndSchedules(t *testing.T) {
	manager, err := New()
	if err != nil {
		t.Fatal(err)
	}
	job := Job{Name: "unique", Schedule: Every{Duration: time.Hour}, Handler: func(context.Context) error { return nil }}
	if err := manager.Add(job); err != nil {
		t.Fatal(err)
	}
	if err := manager.Add(job); !errors.Is(err, ErrDuplicateJob) {
		t.Fatalf("duplicate error = %v", err)
	}
	if err := manager.Add(Job{Name: "bad", Schedule: Every{}, Handler: job.Handler}); err == nil {
		t.Fatal("zero interval was accepted")
	}
	if err := manager.Add(Job{Name: "past", Schedule: Once{At: time.Now().Add(-time.Second)}, Handler: job.Handler}); err == nil {
		t.Fatal("past one-time job was accepted")
	}
	if err := manager.Remove("unique"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove("unique"); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("missing removal error = %v", err)
	}
}

func TestNewAndStartState(t *testing.T) {
	if _, err := New(WithLocation(nil)); err == nil {
		t.Fatal("nil location was accepted")
	}
	manager, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); !errors.Is(err, ErrStarted) {
		t.Fatalf("second start error = %v", err)
	}
	manager.Stop()
	if err := manager.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); !errors.Is(err, ErrStopped) {
		t.Fatalf("start after stop error = %v", err)
	}
}

func TestJobsAreSortedByName(t *testing.T) {
	manager, err := New()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"zulu", "alpha"} {
		if err := manager.Add(Job{Name: name, Schedule: Every{Duration: time.Hour}, Handler: func(context.Context) error { return nil }}); err != nil {
			t.Fatal(err)
		}
	}
	jobs := manager.Jobs()
	if len(jobs) != 2 || jobs[0].Name != "alpha" || jobs[1].Name != "zulu" {
		t.Fatalf("sorted jobs = %#v", jobs)
	}
}

func TestManagerRecoversPanicsAndAppliesTimeout(t *testing.T) {
	logger := &recordingLogger{}
	manager, err := New(WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}
	timedOut := make(chan struct{})
	if err := manager.Add(Job{
		Name:           "panic",
		Schedule:       Every{Duration: time.Hour},
		RunImmediately: true,
		Handler: func(context.Context) error {
			panic("boom")
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Add(Job{
		Name:           "timeout",
		Schedule:       Every{Duration: time.Hour},
		Timeout:        20 * time.Millisecond,
		RunImmediately: true,
		Handler: func(ctx context.Context) error {
			<-ctx.Done()
			close(timedOut)
			return ctx.Err()
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		manager.Stop()
		_ = manager.Wait(context.Background())
	}()
	select {
	case <-timedOut:
	case <-time.After(time.Second):
		t.Fatal("job timeout did not cancel its context")
	}
	if !logger.contains("scheduler job panicked") {
		t.Fatalf("panic was not logged: %#v", logger.messages())
	}
	if !logger.contains("scheduler job failed") {
		t.Fatalf("timeout was not logged: %#v", logger.messages())
	}
}

func TestOverlapGateSkipAndQueueOne(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		policy     OverlapPolicy
		triggers   int
		wantRuns   int
		wantQueued bool
	}{
		{name: "skip", policy: SkipIfRunning, triggers: 1, wantRuns: 1},
		{name: "queue one", policy: QueueOne, triggers: 2, wantRuns: 2, wantQueued: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			gate := overlapGate{policy: testCase.policy}
			entered := make(chan struct{})
			release := make(chan struct{})
			finished := make(chan struct{})
			var runs sync.WaitGroup
			runs.Add(testCase.wantRuns)
			go func() {
				gate.run(func() {
					runs.Done()
					select {
					case <-entered:
					default:
						close(entered)
						<-release
					}
				})
				close(finished)
			}()
			<-entered
			for index := 0; index < testCase.triggers; index++ {
				executed, queued := gate.run(func() { t.Fatal("overlapping trigger executed immediately") })
				if executed {
					t.Fatal("overlapping trigger executed")
				}
				if index == 0 && queued != testCase.wantQueued {
					t.Fatalf("queued = %t, want %t", queued, testCase.wantQueued)
				}
			}
			close(release)
			wait := make(chan struct{})
			go func() { runs.Wait(); close(wait) }()
			select {
			case <-wait:
			case <-time.After(time.Second):
				t.Fatalf("runs did not reach %d", testCase.wantRuns)
			}
			<-finished
		})
	}
}

type recordingLogger struct {
	mu   sync.Mutex
	logs []string
}

func (logger *recordingLogger) Debug(context.Context, string, ...logging.Field) {}

func (logger *recordingLogger) Info(_ context.Context, message string, _ ...logging.Field) {
	logger.record(message)
}

func (logger *recordingLogger) Warn(_ context.Context, message string, _ ...logging.Field) {
	logger.record(message)
}

func (logger *recordingLogger) Error(_ context.Context, message string, _ ...logging.Field) {
	logger.record(message)
}

func (logger *recordingLogger) With(...logging.Field) logging.Logger { return logger }
func (logger *recordingLogger) Named(string) logging.Logger          { return logger }
func (logger *recordingLogger) Enabled(logging.Level) bool           { return true }
func (logger *recordingLogger) Sync() error                          { return nil }

func (logger *recordingLogger) record(message string) {
	logger.mu.Lock()
	logger.logs = append(logger.logs, message)
	logger.mu.Unlock()
}

func (logger *recordingLogger) contains(want string) bool {
	for _, message := range logger.messages() {
		if strings.Contains(message, want) {
			return true
		}
	}
	return false
}

func (logger *recordingLogger) messages() []string {
	logger.mu.Lock()
	defer logger.mu.Unlock()
	return append([]string(nil), logger.logs...)
}

package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"
)

type errorRecorder struct {
	startErr  error
	finishErr error
	started   int
	finished  int
}

func (recorder *errorRecorder) Start(context.Context, RunRecord) error {
	recorder.started++
	return recorder.startErr
}
func (recorder *errorRecorder) Finish(context.Context, RunRecord) error {
	recorder.finished++
	return recorder.finishErr
}

func TestPersistentSingletonLockMissRecordsSkipped(t *testing.T) {
	recorder := &testRecorder{}
	loader, err := NewLoader(&memoryJobStore{}, persistentRegistry(t), newTestScheduler(), WithLocker(&testLocker{acquired: false}), WithRunRecorder(recorder))
	if err != nil { t.Fatal(err) }
	called := false
	definition := persistentDefinition("1", "single", 1)
	definition.ExecutionMode = Singleton
	if err := loader.execute(context.Background(), func(context.Context, Execution) error { called = true; return nil }, definition); err != nil { t.Fatal(err) }
	if called { t.Fatal("handler ran without singleton lock") }
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.records) != 2 || recorder.records[0].Status != RunRunning || recorder.records[1].Status != RunSkipped {
		t.Fatalf("records = %#v", recorder.records)
	}
}

func TestPersistentPanicFinalizesRunRecord(t *testing.T) {
	recorder := &testRecorder{}
	loader, err := NewLoader(&memoryJobStore{}, persistentRegistry(t), newTestScheduler(), WithRunRecorder(recorder))
	if err != nil { t.Fatal(err) }
	definition := persistentDefinition("1", "panic", 1)
	func() {
		defer func() {
			if recover() == nil { t.Fatal("persistent panic was swallowed") }
		}()
		_ = loader.execute(context.Background(), func(context.Context, Execution) error { panic("boom") }, definition)
	}()
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.records) != 2 || recorder.records[1].Status != RunFailed || recorder.records[1].FinishedAt.IsZero() {
		t.Fatalf("records = %#v", recorder.records)
	}
}

func TestPersistentRecorderErrorsDoNotSuppressHandler(t *testing.T) {
	startFailure := errors.New("start failed")
	startRecorder := &errorRecorder{startErr: startFailure}
	loader, err := NewLoader(&memoryJobStore{}, persistentRegistry(t), newTestScheduler(), WithRunRecorder(startRecorder))
	if err != nil { t.Fatal(err) }
	called := false
	if err := loader.execute(context.Background(), func(context.Context, Execution) error { called = true; return nil }, persistentDefinition("1", "job", 1)); !errors.Is(err, startFailure) {
		t.Fatalf("start error = %v", err)
	}
	if !called { t.Fatal("recorder Start failure suppressed business handler") }
	if startRecorder.finished != 1 { t.Fatalf("finish calls = %d", startRecorder.finished) }

	finishFailure := errors.New("finish failed")
	finishRecorder := &errorRecorder{finishErr: finishFailure}
	loader, err = NewLoader(&memoryJobStore{}, persistentRegistry(t), newTestScheduler(), WithRunRecorder(finishRecorder))
	if err != nil { t.Fatal(err) }
	called = false
	if err := loader.execute(context.Background(), func(context.Context, Execution) error { called = true; return nil }, persistentDefinition("2", "job2", 1)); !errors.Is(err, finishFailure) {
		t.Fatalf("finish error = %v", err)
	}
	if !called { t.Fatal("handler did not run") }
}

func TestPersistentExecutionPayloadIsIsolated(t *testing.T) {
	loader, err := NewLoader(&memoryJobStore{}, persistentRegistry(t), newTestScheduler())
	if err != nil { t.Fatal(err) }
	definition := persistentDefinition("1", "payload", 1)
	definition.Payload = []byte(`{"value":1}`)
	original := append([]byte(nil), definition.Payload...)
	if err := loader.execute(context.Background(), func(_ context.Context, execution Execution) error {
		execution.Definition.Payload[0] = 'x'
		return nil
	}, definition); err != nil { t.Fatal(err) }
	if string(definition.Payload) != string(original) {
		t.Fatalf("definition payload mutated: %q", definition.Payload)
	}
}

type typedNilRecorder struct{}
func (*typedNilRecorder) Start(context.Context, RunRecord) error { panic("typed nil recorder called") }
func (*typedNilRecorder) Finish(context.Context, RunRecord) error { panic("typed nil recorder called") }

func TestNewLoaderNormalizesTypedNilRecorder(t *testing.T) {
	var recorder *typedNilRecorder
	loader, err := NewLoader(&memoryJobStore{}, persistentRegistry(t), newTestScheduler(), WithRunRecorder(recorder))
	if err != nil { t.Fatal(err) }
	if loader.recorder != nil { t.Fatal("typed nil recorder was retained") }
	if err := loader.execute(context.Background(), func(context.Context, Execution) error { return nil }, persistentDefinition("1", "job", 1)); err != nil { t.Fatal(err) }
}

func TestPersistentExecutionClassifiesCancellation(t *testing.T) {
	recorder := &testRecorder{}
	loader, err := NewLoader(&memoryJobStore{}, persistentRegistry(t), newTestScheduler(), WithRunRecorder(recorder))
	if err != nil { t.Fatal(err) }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := loader.execute(ctx, func(context.Context, Execution) error { return nil }, persistentDefinition("1", "cancel", 1)); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled execution error = %v", err)
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.records) != 2 || recorder.records[1].Status != RunCanceled {
		t.Fatalf("records = %#v", recorder.records)
	}
}

func TestRunIDsAreUnique(t *testing.T) {
	first, err := newRunID()
	if err != nil { t.Fatal(err) }
	second, err := newRunID()
	if err != nil { t.Fatal(err) }
	if first == second || len(first) < 20 || len(second) < 20 { t.Fatalf("run IDs = %q, %q", first, second) }
}

func TestPersistentLockTTLAddsLeaseGrace(t *testing.T) {
	if got := persistentLockTTL(10 * time.Second); got <= 10*time.Second { t.Fatalf("ttl = %s", got) }
	if got := persistentLockTTL(0); got != 0 { t.Fatalf("zero-timeout ttl = %s", got) }
}

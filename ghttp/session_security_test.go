package ghttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lanechi/gonex/session"
)

type failingDeleteStorage struct {
	values map[string]map[string]any
	failID string
}

func (storage *failingDeleteStorage) Get(_ context.Context, id string) (map[string]any, error) {
	values, ok := storage.values[id]
	if !ok {
		return nil, session.ErrNotFound
	}
	return values, nil
}

func (storage *failingDeleteStorage) Set(_ context.Context, id string, values map[string]any, _ time.Duration) error {
	if storage.values == nil {
		storage.values = make(map[string]map[string]any)
	}
	storage.values[id] = values
	return nil
}

func (storage *failingDeleteStorage) Delete(_ context.Context, id string) error {
	if id == storage.failID {
		return errors.New("delete failed")
	}
	delete(storage.values, id)
	return nil
}

func TestSessionCookieOptionsRejectInsecureSameSiteNone(t *testing.T) {
	manager := NewSessionManager(nil, "", time.Hour)
	before := manager.CookieOptions()
	if err := manager.SetCookieOptions(CookieOptions{Path: "/", SameSite: http.SameSiteNoneMode}); err == nil {
		t.Fatal("expected insecure SameSite=None to be rejected")
	}
	if after := manager.CookieOptions(); after != before {
		t.Fatalf("invalid options mutated manager: before=%+v after=%+v", before, after)
	}
}

func TestSessionRegenerateRollsBackWhenOldIDCannotBeDeleted(t *testing.T) {
	storage := &failingDeleteStorage{values: make(map[string]map[string]any)}
	manager := NewSessionManager(storage, "session_id", time.Hour)

	engine := NewServer(WithSessionManager(manager))
	if err := engine.Err(); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	ginContext, _ := createTestContext(recorder, request)
	ctx := newContext(ginContext)
	ctx.server = engine
	ctx.sessionManager = manager

	opened, err := manager.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	oldID := opened.ID()
	storage.failID = oldID
	if err := opened.Regenerate(); err == nil {
		t.Fatal("expected regenerate failure")
	}
	if opened.ID() != oldID {
		t.Fatalf("session id changed after failed regenerate: got %q want %q", opened.ID(), oldID)
	}
	if _, ok := storage.values[oldID]; !ok {
		t.Fatal("old session disappeared after failed regenerate")
	}
	if len(storage.values) != 1 {
		t.Fatalf("replacement session leaked after rollback: %#v", storage.values)
	}
}

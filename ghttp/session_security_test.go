package ghttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
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

func newSessionTestContext(t *testing.T, manager *SessionManager) *Context {
	t.Helper()
	server := NewServer(WithSessionManager(manager))
	if err := server.Err(); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := newContext(ginContext)
	ctx.server = server
	ctx.sessionManager = manager
	return ctx
}

func TestSessionCookieOptionsEnforceSecureSameSiteNone(t *testing.T) {
	manager := NewSessionManager(nil, "", time.Hour)
	manager.SetCookieOptions(CookieOptions{Path: "/", SameSite: http.SameSiteNoneMode})
	after := manager.CookieOptions()
	if !after.Secure || after.SameSite != http.SameSiteNoneMode {
		t.Fatalf("SameSite=None did not force Secure: %+v", after)
	}
}

func TestSessionRegenerateRollsBackWhenOldIDCannotBeDeleted(t *testing.T) {
	storage := &failingDeleteStorage{values: make(map[string]map[string]any)}
	manager := NewSessionManager(storage, "session_id", time.Hour)
	ctx := newSessionTestContext(t, manager)

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

func TestSessionLogoutKeepsHandleWhenStorageDeleteFails(t *testing.T) {
	storage := &failingDeleteStorage{values: make(map[string]map[string]any)}
	manager := NewSessionManager(storage, "session_id", time.Hour)
	ctx := newSessionTestContext(t, manager)

	opened, err := manager.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	oldID := opened.ID()
	if err := opened.Set("state", "active"); err != nil {
		t.Fatal(err)
	}
	storage.failID = oldID
	if err := opened.Logout(); err == nil {
		t.Fatal("expected logout failure")
	}
	if opened.ID() != oldID {
		t.Fatalf("session id changed after failed logout: got %q want %q", opened.ID(), oldID)
	}
	value, err := opened.Get("state")
	if err != nil {
		t.Fatal(err)
	}
	if value != "active" {
		t.Fatalf("session state lost after failed logout: %#v", value)
	}
	if err := opened.Set("retry", true); err != nil {
		t.Fatalf("failed logout incorrectly marked session logged out: %v", err)
	}
}

func TestSessionLogoutPreflightsCookieBeforeRevocation(t *testing.T) {
	revocations := session.NewMemoryCookieRevocationStore()
	storage, err := session.NewCookieStorage(make([]byte, 32), revocations)
	if err != nil {
		t.Fatal(err)
	}
	manager := NewSessionManager(storage, "session_id", time.Hour)
	ctx := newSessionTestContext(t, manager)

	opened, err := manager.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	oldID := opened.ID()
	ctx.gin.Writer.WriteHeader(http.StatusOK)

	if err := opened.Logout(); err == nil {
		t.Fatal("expected logout failure after response headers were written")
	}
	if opened.ID() != oldID {
		t.Fatalf("session id changed after unpublishable logout: got %q want %q", opened.ID(), oldID)
	}
	if _, err := storage.Get(context.Background(), oldID); err != nil {
		t.Fatalf("logout revoked token before cookie deletion preflight: %v", err)
	}
}

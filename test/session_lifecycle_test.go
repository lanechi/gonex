package ghttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lanechi/gonex/g"
	"github.com/lanechi/gonex/ghttp"
	"github.com/lanechi/gonex/session"
)

type sessionOperationRequest struct {
	g.Meta `path:"/session-operation" method:"post"`
	Action string `query:"action" binding:"required"`
}

type sessionOperationResponse struct {
	ID    string `json:"id"`
	Value any    `json:"value,omitempty"`
}

type sessionOperationController struct{}

func (*sessionOperationController) Execute(ctx context.Context, request *sessionOperationRequest) (*sessionOperationResponse, error) {
	frameworkContext := ghttp.FromContext(ctx)
	currentSession, err := frameworkContext.Session()
	if err != nil {
		return nil, err
	}
	switch request.Action {
	case "set":
		err = currentSession.Set("value", "stored")
	case "delete":
		err = currentSession.Delete("value")
	case "clear":
		err = currentSession.Clear()
	case "regenerate":
		err = currentSession.Regenerate()
	case "logout":
		err = currentSession.Logout()
	case "logout-reopen":
		previous := currentSession
		if err = previous.Logout(); err == nil {
			currentSession, err = frameworkContext.Session()
		}
		if err == nil && currentSession == previous {
			err = errors.New("request context returned the logged-out session")
		}
		if err == nil {
			err = currentSession.Set("value", "reopened")
		}
	}
	if err != nil {
		return nil, err
	}
	value, err := currentSession.Get("value")
	if err != nil {
		return nil, err
	}
	return &sessionOperationResponse{ID: currentSession.ID(), Value: value}, nil
}

func TestSessionLogoutReopensWithinSameRequest(t *testing.T) {
	tests := []struct {
		name    string
		storage session.Storage
	}{
		{name: "memory", storage: session.NewMemoryStorage()},
		{name: "cookie", storage: mustCookieStorage(t, "logout-reopen-cookie-secret-at-least-32-bytes")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := ghttp.NewServer(
				ghttp.WithLogger(&recordingLogger{}),
				ghttp.WithSessionManager(ghttp.NewSessionManager(test.storage, "sid", time.Hour)),
			)
			if err := server.Bind(&sessionOperationController{}); err != nil {
				t.Fatal(err)
			}

			_, original := sessionOperationRequestToServer(t, server, "set", nil)
			response, reopened := sessionOperationRequestToServer(t, server, "logout-reopen", original)
			if response.Code != http.StatusOK || reopened == nil || reopened.Value == "" || reopened.Value == original.Value ||
				!strings.Contains(response.Body.String(), `"value":"reopened"`) {
				t.Fatalf("reopen response: status=%d original=%#v reopened=%#v body=%s", response.Code, original, reopened, response.Body.String())
			}
			next, _ := sessionOperationRequestToServer(t, server, "get", reopened)
			if next.Code != http.StatusOK || !strings.Contains(next.Body.String(), `"value":"reopened"`) {
				t.Fatalf("reopened session did not persist: status=%d body=%s", next.Code, next.Body.String())
			}
		})
	}
}

func TestMemorySessionRegenerateDeleteClearAndLogout(t *testing.T) {
	storage := session.NewMemoryStorage()
	server := ghttp.NewServer(
		ghttp.WithLogger(&recordingLogger{}),
		ghttp.WithSessionManager(ghttp.NewSessionManager(storage, "sid", time.Hour)),
		ghttp.WithSessionCookieOptions(ghttp.CookieOptions{Path: "/", HTTPOnly: true, SameSite: http.SameSiteStrictMode}),
	)
	if err := server.Bind(&sessionOperationController{}); err != nil {
		t.Fatal(err)
	}

	response, cookie := sessionOperationRequestToServer(t, server, "set", nil)
	if response.Code != http.StatusOK || cookie == nil || !strings.Contains(response.Body.String(), `"value":"stored"`) || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("set response: status=%d cookie=%#v body=%s", response.Code, cookie, response.Body.String())
	}
	oldID := cookie.Value
	response, regeneratedCookie := sessionOperationRequestToServer(t, server, "regenerate", cookie)
	if response.Code != http.StatusOK || regeneratedCookie == nil || regeneratedCookie.Value == oldID {
		t.Fatalf("regenerate response: status=%d old=%q cookie=%#v body=%s", response.Code, oldID, regeneratedCookie, response.Body.String())
	}
	if _, err := storage.Get(context.Background(), oldID); err != session.ErrNotFound {
		t.Fatalf("old memory session was not removed: %v", err)
	}

	response, regeneratedCookie = sessionOperationRequestToServer(t, server, "delete", regeneratedCookie)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), `"value"`) {
		t.Fatalf("delete response: status=%d body=%s", response.Code, response.Body.String())
	}
	_, regeneratedCookie = sessionOperationRequestToServer(t, server, "set", regeneratedCookie)
	response, regeneratedCookie = sessionOperationRequestToServer(t, server, "clear", regeneratedCookie)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), `"value"`) {
		t.Fatalf("clear response: status=%d body=%s", response.Code, response.Body.String())
	}
	response, logoutCookie := sessionOperationRequestToServer(t, server, "logout", regeneratedCookie)
	if response.Code != http.StatusOK || logoutCookie == nil || logoutCookie.MaxAge >= 0 {
		t.Fatalf("logout response: status=%d cookie=%#v body=%s", response.Code, logoutCookie, response.Body.String())
	}
}

func TestCookieSessionRegenerateRevokesOldCookie(t *testing.T) {
	storage := mustCookieStorage(t, "cookie-session-test-secret-at-least-32-bytes")
	server := ghttp.NewServer(ghttp.WithLogger(&recordingLogger{}), ghttp.WithSessionManager(ghttp.NewSessionManager(storage, "sid", time.Hour)))
	if err := server.Bind(&sessionOperationController{}); err != nil {
		t.Fatal(err)
	}

	_, oldCookie := sessionOperationRequestToServer(t, server, "set", nil)
	if oldCookie == nil {
		t.Fatal("signed session cookie is missing")
	}
	_, newCookie := sessionOperationRequestToServer(t, server, "regenerate", oldCookie)
	if newCookie == nil || newCookie.Value == oldCookie.Value {
		t.Fatalf("signed cookie was not regenerated: old=%#v new=%#v", oldCookie, newCookie)
	}
	if _, err := storage.Get(context.Background(), oldCookie.Value); err != session.ErrNotFound {
		t.Fatalf("old signed cookie can still be replayed: %v", err)
	}
	if values, err := storage.Get(context.Background(), newCookie.Value); err != nil || values["value"] != "stored" {
		t.Fatalf("new signed cookie values=%#v err=%v", values, err)
	}
	_, logoutCookie := sessionOperationRequestToServer(t, server, "logout", newCookie)
	if _, err := storage.Get(context.Background(), newCookie.Value); err != session.ErrNotFound {
		t.Fatalf("logged-out signed cookie can still be replayed: %v", err)
	}
	if logoutCookie == nil || logoutCookie.MaxAge >= 0 {
		t.Fatalf("logout cookie=%#v", logoutCookie)
	}
}

func TestCookieSessionLogoutRevokesRotatedTokenFamily(t *testing.T) {
	storage := mustCookieStorage(t, "cookie-session-family-test-secret-at-least-32-bytes")
	server := ghttp.NewServer(ghttp.WithLogger(&recordingLogger{}), ghttp.WithSessionManager(ghttp.NewSessionManager(storage, "sid", time.Hour)))
	if err := server.Bind(&sessionOperationController{}); err != nil {
		t.Fatal(err)
	}

	_, tokenA := sessionOperationRequestToServer(t, server, "set", nil)
	_, tokenB := sessionOperationRequestToServer(t, server, "delete", tokenA)
	if tokenA == nil || tokenB == nil || tokenA.Value == tokenB.Value {
		t.Fatalf("cookie rotation failed: A=%#v B=%#v", tokenA, tokenB)
	}
	_, _ = sessionOperationRequestToServer(t, server, "logout", tokenB)
	if _, err := storage.Get(context.Background(), tokenA.Value); err != session.ErrNotFound {
		t.Fatalf("replayed pre-rotation token remained valid after logout: %v", err)
	}
	if _, err := storage.Get(context.Background(), tokenB.Value); err != session.ErrNotFound {
		t.Fatalf("replayed current token remained valid after logout: %v", err)
	}

	_, independent := sessionOperationRequestToServer(t, server, "set", nil)
	if values, err := storage.Get(context.Background(), independent.Value); err != nil || values["value"] != "stored" {
		t.Fatalf("logout revoked an independent session: values=%#v err=%v", values, err)
	}
}

func TestCookieSessionLogoutCoversInFlightFamilyRotations(t *testing.T) {
	storage := mustCookieStorage(t, "cookie-session-in-flight-family-secret-at-least-32-bytes")
	tokenA, err := storage.Encode(map[string]any{"value": "first"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	family := storage.Family(tokenA)
	tokenB, err := storage.EncodeWithFamily(map[string]any{"value": "in-flight"}, 2*time.Hour, family)
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.Delete(context.Background(), tokenA); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Get(context.Background(), tokenB); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("in-flight token remained valid after logout: %v", err)
	}
	if _, err := storage.EncodeWithFamily(map[string]any{"value": "late"}, 2*time.Hour, family); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("revoked family accepted a late rotation: %v", err)
	}
}

func TestContextSessionCachesOneInstancePerRequest(t *testing.T) {
	storage := session.NewMemoryStorage()
	manager := ghttp.NewSessionManager(storage, "sid", time.Hour)
	server := ghttp.NewServer(ghttp.WithLogger(&recordingLogger{}), ghttp.WithSessionManager(manager))
	if err := server.Use(func(ginContext *gin.Context) {
		frameworkContext := ghttp.FromContext(ginContext.Request.Context())
		current, sessionErr := frameworkContext.Session()
		if sessionErr != nil {
			ginContext.Error(sessionErr)
			ginContext.Abort()
			return
		}
		if sessionErr := current.Set("value", "middleware"); sessionErr != nil {
			ginContext.Error(sessionErr)
			ginContext.Abort()
			return
		}
		ginContext.Set("middleware-session", current)
		ginContext.Next()
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.Bind(&sessionReuseController{}); err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/session-reuse", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"same":true`) || !strings.Contains(response.Body.String(), `"value":"middleware"`) {
		t.Fatalf("cached session response: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestSessionStorageReceivesRequestContext(t *testing.T) {
	storage := &contextAwareSessionStorage{MemoryStorage: session.NewMemoryStorage()}
	server := ghttp.NewServer(ghttp.WithLogger(&recordingLogger{}), ghttp.WithSessionManager(ghttp.NewSessionManager(storage, "sid", time.Hour)))
	if err := server.Bind(&sessionReuseController{}); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/session-reuse", nil)
	request = request.WithContext(context.WithValue(request.Context(), sessionContextKey{}, "request-value"))
	server.ServeHTTP(httptest.NewRecorder(), request)
	if storage.lastContext == nil || storage.lastContext.Value(sessionContextKey{}) != "request-value" {
		t.Fatalf("storage context=%v", storage.lastContext)
	}
}

type sessionContextKey struct{}

type contextAwareSessionStorage struct {
	*session.MemoryStorage
	lastContext context.Context
}

func (storage *contextAwareSessionStorage) Get(ctx context.Context, id string) (map[string]any, error) {
	storage.lastContext = ctx
	return storage.MemoryStorage.Get(ctx, id)
}

func (storage *contextAwareSessionStorage) Set(ctx context.Context, id string, values map[string]any, ttl time.Duration) error {
	storage.lastContext = ctx
	return storage.MemoryStorage.Set(ctx, id, values, ttl)
}

func (storage *contextAwareSessionStorage) Delete(ctx context.Context, id string) error {
	storage.lastContext = ctx
	return storage.MemoryStorage.Delete(ctx, id)
}

type sessionReuseRequest struct {
	g.Meta `path:"/session-reuse" method:"get"`
}

type sessionReuseController struct{}

type sessionReuseResponse struct {
	Same  bool `json:"same"`
	Value any  `json:"value"`
}

func (*sessionReuseController) Get(ctx context.Context, _ *sessionReuseRequest) (*sessionReuseResponse, error) {
	frameworkContext := ghttp.FromContext(ctx)
	first, err := frameworkContext.Session()
	if err != nil {
		return nil, err
	}
	second, err := frameworkContext.Session()
	if err != nil {
		return nil, err
	}
	value, err := second.Get("value")
	if err != nil {
		return nil, err
	}
	middlewareSession, _ := frameworkContext.Gin().Get("middleware-session")
	return &sessionReuseResponse{Same: first == second && first == middlewareSession, Value: value}, nil
}

func sessionOperationRequestToServer(t *testing.T, server *ghttp.Server, action string, cookie *http.Cookie) (*httptest.ResponseRecorder, *http.Cookie) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/session-operation?action="+action, nil)
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	var latest *http.Cookie
	for _, responseCookie := range response.Result().Cookies() {
		if responseCookie.Name == "sid" {
			latest = responseCookie
		}
	}
	if latest == nil {
		latest = cookie
	}
	return response, latest
}

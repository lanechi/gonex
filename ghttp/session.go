package ghttp

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/lanechi/gonex/internal/sessionvalue"
	"github.com/lanechi/gonex/session"
)

// SessionManager owns session cookies and delegates persistence to a storage
// backend.
type SessionManager struct {
	storage       session.Storage
	cookieName    string
	ttl           time.Duration
	cookieMu      sync.RWMutex
	cookieOptions CookieOptions
}

// NewSessionManager creates an HTTP session manager over storage. Storage I/O
// receives each request context; the application retains ownership of storage
// and any external client it uses. Nil and typed-nil storage both select the
// process-local memory backend.
func NewSessionManager(storage session.Storage, cookieName string, ttl time.Duration) *SessionManager {
	if isNilInterface(storage) {
		storage = session.NewMemoryStorage()
	}
	if cookieName == "" {
		cookieName = "session_id"
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &SessionManager{
		storage: storage, cookieName: cookieName, ttl: ttl,
		cookieOptions: CookieOptions{Path: "/", MaxAge: int(ttl.Seconds()), HTTPOnly: true, SameSite: http.SameSiteLaxMode},
	}
}

func secureSessionCookieOptions(options CookieOptions) CookieOptions {
	if options.SameSite == http.SameSiteNoneMode {
		options.Secure = true
	}
	return options
}

// SetCookieOptions replaces the flags used for the session identifier cookie.
// SameSite=None always implies Secure so runtime mutation cannot weaken the
// framework's session-cookie invariant.
func (manager *SessionManager) SetCookieOptions(options CookieOptions) {
	if manager == nil {
		return
	}
	options = secureSessionCookieOptions(options)
	manager.cookieMu.Lock()
	manager.cookieOptions = options
	manager.cookieMu.Unlock()
}

// CookieOptions returns a copy of the session identifier cookie options.
func (manager *SessionManager) CookieOptions() CookieOptions {
	if manager == nil {
		return CookieOptions{}
	}
	manager.cookieMu.RLock()
	defer manager.cookieMu.RUnlock()
	return manager.cookieOptions
}

func (manager *SessionManager) Open(ctx *Context) (session.Session, error) {
	if manager == nil || ctx == nil {
		return nil, errors.New("session manager is not configured")
	}
	id, err := ctx.Cookie().Get(manager.cookieName)
	newID := err != nil || id == ""
	createdPersistentSession := false
	values := make(map[string]any)
	if !newID {
		values, err = manager.get(ctx, id)
		if errors.Is(err, session.ErrNotFound) {
			values = make(map[string]any)
			newID = true
		} else if err != nil {
			return nil, err
		} else {
			values, err = sessionvalue.NormalizeMap(values)
			if err != nil {
				return nil, err
			}
		}
	}
	if newID {
		if cookieStorage, ok := manager.storage.(*session.CookieStorage); ok {
			id, err = cookieStorage.Encode(values, manager.ttl)
			if err != nil {
				return nil, err
			}
		} else {
			id, err = session.NewID()
			if err != nil {
				return nil, err
			}
			if err := manager.set(ctx, id, values); err != nil {
				return nil, err
			}
			createdPersistentSession = true
		}
	}
	if id == "" {
		return nil, session.ErrNotFound
	}
	if err := ctx.Cookie().Set(manager.cookieName, id, manager.CookieOptions()); err != nil {
		if createdPersistentSession {
			_ = manager.delete(ctx, id)
		}
		return nil, err
	}
	managed := &managedSession{manager: manager, context: ctx, id: id, values: values}
	if cookieStorage, ok := manager.storage.(*session.CookieStorage); ok {
		managed.family = cookieStorage.Family(id)
	}
	return managed, nil
}

func (manager *SessionManager) persist(current *managedSession) error {
	if cookieStorage, ok := manager.storage.(*session.CookieStorage); ok {
		oldID := current.id
		id, err := cookieStorage.EncodeWithFamily(current.values, manager.ttl, current.family)
		if err != nil {
			return err
		}
		if err := current.context.Cookie().Set(manager.cookieName, id, manager.CookieOptions()); err != nil {
			return err
		}
		current.id = id
		current.family = cookieStorage.Family(id)
		return cookieStorage.RevokeToken(oldID)
	}
	return manager.set(current.context, current.id, current.values)
}

func (manager *SessionManager) regenerate(current *managedSession) error {
	if cookieStorage, ok := manager.storage.(*session.CookieStorage); ok {
		oldID := current.id
		id, err := cookieStorage.EncodeWithFamily(current.values, manager.ttl, current.family)
		if err != nil {
			return err
		}
		if err := current.context.Cookie().Set(manager.cookieName, id, manager.CookieOptions()); err != nil {
			return err
		}
		current.id = id
		current.family = cookieStorage.Family(id)
		return cookieStorage.RevokeToken(oldID)
	}
	oldID := current.id
	newID, err := session.NewID()
	if err != nil {
		return err
	}
	if err := manager.set(current.context, newID, current.values); err != nil {
		return err
	}
	if err := current.context.Cookie().Set(manager.cookieName, newID, manager.CookieOptions()); err != nil {
		_ = manager.delete(current.context, newID)
		return err
	}
	if err := manager.delete(current.context, oldID); err != nil {
		cleanupErr := manager.delete(current.context, newID)
		restoreErr := current.context.Cookie().Set(manager.cookieName, oldID, manager.CookieOptions())
		return errors.Join(err, cleanupErr, restoreErr)
	}
	current.id = newID
	return nil
}

func (manager *SessionManager) requestContext(ctx *Context) context.Context {
	if ctx != nil && ctx.Request() != nil {
		return ctx.Request().Context()
	}
	return context.Background()
}

func (manager *SessionManager) get(ctx *Context, id string) (map[string]any, error) {
	return manager.storage.Get(manager.requestContext(ctx), id)
}

func (manager *SessionManager) set(ctx *Context, id string, values map[string]any) error {
	return manager.storage.Set(manager.requestContext(ctx), id, values, manager.ttl)
}

func (manager *SessionManager) delete(ctx *Context, id string) error {
	return manager.storage.Delete(manager.requestContext(ctx), id)
}

type managedSession struct {
	manager   *SessionManager
	context   *Context
	mu        sync.RWMutex
	id        string
	family    string
	values    map[string]any
	loggedOut bool
}

func (current *managedSession) Get(key string) (any, error) {
	current.mu.RLock()
	defer current.mu.RUnlock()
	if current.loggedOut {
		return nil, nil
	}
	value, ok := current.values[key]
	if !ok {
		return nil, nil
	}
	return sessionvalue.Clone(value), nil
}

func (current *managedSession) Set(key string, value any) error {
	normalized, err := sessionvalue.Normalize(value)
	if err != nil {
		return err
	}
	current.mu.Lock()
	defer current.mu.Unlock()
	if current.loggedOut {
		return errors.New("session is logged out; reopen it from the request context")
	}
	previous, existed := current.values[key]
	current.values[key] = normalized
	if err := current.manager.persist(current); err != nil {
		if existed {
			current.values[key] = previous
		} else {
			delete(current.values, key)
		}
		return err
	}
	return nil
}

func (current *managedSession) Delete(key string) error {
	current.mu.Lock()
	defer current.mu.Unlock()
	if current.loggedOut {
		return errors.New("session is logged out; reopen it from the request context")
	}
	previous, existed := current.values[key]
	delete(current.values, key)
	if err := current.manager.persist(current); err != nil {
		if existed {
			current.values[key] = previous
		}
		return err
	}
	return nil
}

func (current *managedSession) Clear() error {
	current.mu.Lock()
	defer current.mu.Unlock()
	if current.loggedOut {
		return errors.New("session is logged out; reopen it from the request context")
	}
	previous := current.values
	current.values = make(map[string]any)
	if err := current.manager.persist(current); err != nil {
		current.values = previous
		return err
	}
	return nil
}

func (current *managedSession) ID() string {
	current.mu.RLock()
	defer current.mu.RUnlock()
	return current.id
}

func (current *managedSession) Regenerate() error {
	current.mu.Lock()
	defer current.mu.Unlock()
	if current.loggedOut {
		return errors.New("session is logged out; reopen it from the request context")
	}
	return current.manager.regenerate(current)
}

func (current *managedSession) Logout() error {
	current.mu.Lock()
	defer current.mu.Unlock()
	if current.loggedOut {
		return nil
	}
	storageErr := current.manager.delete(current.context, current.id)
	cookieErr := current.context.Cookie().Delete(current.manager.cookieName, current.manager.CookieOptions())
	current.values = make(map[string]any)
	current.id = ""
	current.family = ""
	current.loggedOut = true
	current.context.evictSession(current)
	return errors.Join(storageErr, cookieErr)
}

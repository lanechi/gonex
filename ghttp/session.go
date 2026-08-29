package ghttp

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

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
// and any external client it uses.
func NewSessionManager(storage session.Storage, cookieName string, ttl time.Duration) *SessionManager {
	if storage == nil {
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

// SetCookieOptions replaces the flags used for the session identifier cookie.
func (manager *SessionManager) SetCookieOptions(options CookieOptions) {
	if manager != nil {
		manager.cookieMu.Lock()
		defer manager.cookieMu.Unlock()
		manager.cookieOptions = options
	}
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
	current.id = newID
	return manager.delete(current.context, oldID)
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

func (session *managedSession) Get(key string) (any, error) {
	session.mu.RLock()
	defer session.mu.RUnlock()
	if session.loggedOut {
		return nil, nil
	}
	value, ok := session.values[key]
	if !ok {
		return nil, nil
	}
	return value, nil
}

func (session *managedSession) Set(key string, value any) error {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.loggedOut {
		return errors.New("session is logged out; reopen it from the request context")
	}
	previous, existed := session.values[key]
	session.values[key] = value
	if err := session.manager.persist(session); err != nil {
		if existed {
			session.values[key] = previous
		} else {
			delete(session.values, key)
		}
		return err
	}
	return nil
}
func (session *managedSession) Delete(key string) error {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.loggedOut {
		return errors.New("session is logged out; reopen it from the request context")
	}
	previous, existed := session.values[key]
	delete(session.values, key)
	if err := session.manager.persist(session); err != nil {
		if existed {
			session.values[key] = previous
		}
		return err
	}
	return nil
}
func (session *managedSession) Clear() error {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.loggedOut {
		return errors.New("session is logged out; reopen it from the request context")
	}
	previous := session.values
	session.values = make(map[string]any)
	if err := session.manager.persist(session); err != nil {
		session.values = previous
		return err
	}
	return nil
}
func (session *managedSession) ID() string {
	session.mu.RLock()
	defer session.mu.RUnlock()
	return session.id
}
func (session *managedSession) Regenerate() error {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.loggedOut {
		return errors.New("session is logged out; reopen it from the request context")
	}
	return session.manager.regenerate(session)
}
func (session *managedSession) Logout() error {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.loggedOut {
		return nil
	}
	storageErr := session.manager.delete(session.context, session.id)
	cookieErr := session.context.Cookie().Delete(session.manager.cookieName, session.manager.CookieOptions())
	session.values = make(map[string]any)
	session.id = ""
	session.family = ""
	session.loggedOut = true
	session.context.evictSession(session)
	return errors.Join(storageErr, cookieErr)
}

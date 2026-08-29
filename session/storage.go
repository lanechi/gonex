// Package session provides session contracts and storage backends.
package session

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"
)

var ErrNotFound = errors.New("session not found")

// Session is the application-facing session contract.
type Session interface {
	Get(key string) (any, error)
	Set(key string, value any) error
	Delete(key string) error
	Clear() error
	ID() string
	Regenerate() error
	Logout() error
}

// Storage is the context-first persistence boundary for session
// implementations. Implementations must not retain ctx after an operation
// returns.
type Storage interface {
	Get(ctx context.Context, id string) (map[string]any, error)
	Set(ctx context.Context, id string, values map[string]any, ttl time.Duration) error
	Delete(ctx context.Context, id string) error
}

type memorySessionEntry struct {
	values    map[string]any
	expiresAt time.Time
}

// MemoryStorage is a process-local session store.
type MemoryStorage struct {
	mu      sync.RWMutex
	entries map[string]memorySessionEntry
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{entries: make(map[string]memorySessionEntry)}
}

func (storage *MemoryStorage) Get(_ context.Context, id string) (map[string]any, error) {
	storage.mu.RLock()
	entry, ok := storage.entries[id]
	storage.mu.RUnlock()
	if !ok || (!entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt)) {
		if ok {
			_ = storage.Delete(context.Background(), id)
		}
		return nil, ErrNotFound
	}
	return cloneValues(entry.values), nil
}

func (storage *MemoryStorage) Set(_ context.Context, id string, values map[string]any, ttl time.Duration) error {
	entry := memorySessionEntry{values: cloneValues(values)}
	if ttl > 0 {
		entry.expiresAt = time.Now().Add(ttl)
	}
	storage.mu.Lock()
	storage.entries[id] = entry
	storage.mu.Unlock()
	return nil
}

func (storage *MemoryStorage) Delete(_ context.Context, id string) error {
	storage.mu.Lock()
	delete(storage.entries, id)
	storage.mu.Unlock()
	return nil
}

// CookieStorage stores signed, JSON-encoded session values in the cookie.
// Revoked tokens are retained until expiry so Regenerate and Logout cannot
// replay an older cookie within the current process.
type CookieStorage struct {
	secret   []byte
	mu       sync.RWMutex
	revoked  map[[sha256.Size]byte]int64
	families map[[sha256.Size]byte]int64
	// familyExpiries tracks the latest token issued for each family so Logout
	// also covers tokens produced by requests that were already in flight.
	familyExpiries map[[sha256.Size]byte]int64
}

func NewCookieStorage(secret []byte) *CookieStorage {
	return &CookieStorage{
		secret:         append([]byte(nil), secret...),
		revoked:        make(map[[sha256.Size]byte]int64),
		families:       make(map[[sha256.Size]byte]int64),
		familyExpiries: make(map[[sha256.Size]byte]int64),
	}
}

type cookiePayload struct {
	ExpiresAt int64          `json:"exp,omitempty"`
	Nonce     string         `json:"nonce,omitempty"`
	Family    string         `json:"family,omitempty"`
	Values    map[string]any `json:"values"`
}

func (storage *CookieStorage) Get(_ context.Context, id string) (map[string]any, error) {
	payload, err := storage.decode(id)
	if err != nil {
		return nil, ErrNotFound
	}
	if storage.isRevoked(id) || storage.isFamilyRevoked(payload.Family) {
		return nil, ErrNotFound
	}
	return cloneValues(payload.Values), nil
}

func (storage *CookieStorage) decode(id string) (cookiePayload, error) {
	if storage == nil || len(storage.secret) == 0 || id == "" {
		return cookiePayload{}, ErrNotFound
	}
	decoded, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil || len(decoded) < sha256.Size {
		return cookiePayload{}, ErrNotFound
	}
	body, signature := decoded[:len(decoded)-sha256.Size], decoded[len(decoded)-sha256.Size:]
	if !hmac.Equal(signature, storage.sign(body)) {
		return cookiePayload{}, ErrNotFound
	}
	var payload cookiePayload
	if err := json.Unmarshal(body, &payload); err != nil || (payload.ExpiresAt > 0 && time.Now().Unix() >= payload.ExpiresAt) {
		return cookiePayload{}, ErrNotFound
	}
	return payload, nil
}

func (storage *CookieStorage) Set(context.Context, string, map[string]any, time.Duration) error {
	return errors.New("cookie session storage must be persisted through Session")
}
func (storage *CookieStorage) Delete(_ context.Context, id string) error {
	payload, err := storage.decode(id)
	if err != nil {
		return nil
	}
	expiresAt := cookieExpiry(payload)
	now := time.Now().Unix()
	key := sha256.Sum256([]byte(id))
	storage.mu.Lock()
	storage.cleanupLocked(now)
	storage.revoked[key] = expiresAt
	if payload.Family != "" {
		familyKey := sha256.Sum256([]byte(payload.Family))
		if familyExpiry := storage.familyExpiries[familyKey]; familyExpiry > expiresAt {
			expiresAt = familyExpiry
		}
		storage.families[familyKey] = expiresAt
	}
	storage.mu.Unlock()
	return nil
}

// RevokeToken invalidates one signed cookie while leaving newer tokens in its
// session family usable. It is used during token rotation.
func (storage *CookieStorage) RevokeToken(id string) error {
	payload, err := storage.decode(id)
	if err != nil {
		return nil
	}
	expiresAt := cookieExpiry(payload)
	now := time.Now().Unix()
	storage.mu.Lock()
	storage.cleanupLocked(now)
	storage.revoked[sha256.Sum256([]byte(id))] = expiresAt
	storage.mu.Unlock()
	return nil
}

func (storage *CookieStorage) isRevoked(id string) bool {
	key := sha256.Sum256([]byte(id))
	storage.mu.RLock()
	expiresAt, revoked := storage.revoked[key]
	storage.mu.RUnlock()
	if !revoked {
		return false
	}
	if expiresAt > time.Now().Unix() {
		return true
	}
	storage.mu.Lock()
	if current, ok := storage.revoked[key]; ok && current <= time.Now().Unix() {
		delete(storage.revoked, key)
	}
	storage.mu.Unlock()
	return false
}

func (storage *CookieStorage) isFamilyRevoked(family string) bool {
	if family == "" {
		return false
	}
	key := sha256.Sum256([]byte(family))
	storage.mu.RLock()
	expiresAt, revoked := storage.families[key]
	storage.mu.RUnlock()
	if !revoked {
		return false
	}
	if expiresAt > time.Now().Unix() {
		return true
	}
	storage.mu.Lock()
	if current, ok := storage.families[key]; ok && current <= time.Now().Unix() {
		delete(storage.families, key)
	}
	storage.mu.Unlock()
	return false
}

func (storage *CookieStorage) cleanupLocked(now int64) {
	for key, expiry := range storage.revoked {
		if expiry <= now {
			delete(storage.revoked, key)
		}
	}
	for key, expiry := range storage.families {
		if expiry <= now {
			delete(storage.families, key)
		}
	}
	for key, expiry := range storage.familyExpiries {
		if expiry <= now {
			delete(storage.familyExpiries, key)
		}
	}
}

// Encode creates a signed cookie value for SessionManager.
func (storage *CookieStorage) Encode(values map[string]any, ttl time.Duration) (string, error) {
	return storage.EncodeWithFamily(values, ttl, "")
}

// EncodeWithFamily creates a signed cookie value in family. An empty family
// starts a new independent session family.
func (storage *CookieStorage) EncodeWithFamily(values map[string]any, ttl time.Duration, family string) (string, error) {
	if storage == nil || len(storage.secret) == 0 {
		return "", errors.New("cookie session secret is required")
	}
	nonce, err := NewID()
	if err != nil {
		return "", err
	}
	if family == "" {
		family, err = NewID()
		if err != nil {
			return "", err
		}
	}
	payload := cookiePayload{Nonce: nonce, Family: family, Values: cloneValues(values)}
	if ttl > 0 {
		payload.ExpiresAt = time.Now().Add(ttl).Unix()
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	body = append(body, storage.sign(body)...)
	familyKey := sha256.Sum256([]byte(family))
	now := time.Now().Unix()
	storage.mu.Lock()
	storage.cleanupLocked(now)
	if revokedUntil := storage.families[familyKey]; revokedUntil > now {
		storage.mu.Unlock()
		return "", ErrNotFound
	}
	if expiresAt := cookieExpiry(payload); expiresAt > storage.familyExpiries[familyKey] {
		storage.familyExpiries[familyKey] = expiresAt
	}
	storage.mu.Unlock()
	return base64.RawURLEncoding.EncodeToString(body), nil
}

func cookieExpiry(payload cookiePayload) int64 {
	if payload.ExpiresAt > 0 {
		return payload.ExpiresAt
	}
	return int64(^uint64(0) >> 1)
}

// Family returns the signed cookie's session family, or an empty string for a
// legacy token that predates session families.
func (storage *CookieStorage) Family(id string) string {
	payload, err := storage.decode(id)
	if err != nil {
		return ""
	}
	return payload.Family
}

func (storage *CookieStorage) sign(value []byte) []byte {
	hasher := hmac.New(sha256.New, storage.secret)
	_, _ = hasher.Write(value)
	return hasher.Sum(nil)
}

func cloneValues(values map[string]any) map[string]any {
	clone := make(map[string]any, len(values))
	for key, value := range values {
		clone[key] = cloneSessionValue(reflect.ValueOf(value))
	}
	return clone
}

func cloneSessionValue(value reflect.Value) any {
	if !value.IsValid() {
		return nil
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return nil
		}
		return cloneSessionValue(value.Elem())
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type()).Interface()
		}
		clone := reflect.New(value.Type().Elem())
		cloned := reflect.ValueOf(cloneSessionValue(value.Elem()))
		if cloned.IsValid() && cloned.Type().AssignableTo(value.Type().Elem()) {
			clone.Elem().Set(cloned)
		} else {
			clone.Elem().Set(value.Elem())
		}
		return clone.Interface()
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type()).Interface()
		}
		clone := reflect.MakeMapWithSize(value.Type(), value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			mapValue := iterator.Value()
			clonedAny := cloneSessionValue(mapValue)
			cloned := reflect.ValueOf(clonedAny)
			if !cloned.IsValid() {
				cloned = reflect.Zero(mapValue.Type())
			} else if !cloned.Type().AssignableTo(mapValue.Type()) {
				cloned = mapValue
			}
			clone.SetMapIndex(iterator.Key(), cloned)
		}
		return clone.Interface()
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type()).Interface()
		}
		clone := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for index := 0; index < value.Len(); index++ {
			clonedAny := cloneSessionValue(value.Index(index))
			cloned := reflect.ValueOf(clonedAny)
			if cloned.IsValid() && cloned.Type().AssignableTo(value.Index(index).Type()) {
				clone.Index(index).Set(cloned)
			} else {
				clone.Index(index).Set(value.Index(index))
			}
		}
		return clone.Interface()
	case reflect.Array:
		clone := reflect.New(value.Type()).Elem()
		for index := 0; index < value.Len(); index++ {
			clonedAny := cloneSessionValue(value.Index(index))
			cloned := reflect.ValueOf(clonedAny)
			if cloned.IsValid() && cloned.Type().AssignableTo(value.Index(index).Type()) {
				clone.Index(index).Set(cloned)
			} else {
				clone.Index(index).Set(value.Index(index))
			}
		}
		return clone.Interface()
	default:
		return value.Interface()
	}
}

// NewID returns a cryptographically random identifier suitable for sessions
// and signed-cookie nonces.
func NewID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate secure session identifier: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}

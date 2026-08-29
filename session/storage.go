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

	"github.com/lanechi/gonex/internal/sessionvalue"
)

var ErrNotFound = errors.New("session not found")

const minimumCookieSecretBytes = 32
const memoryCleanupInterval = time.Minute

// Session is the application-facing session contract. Values must be JSON-safe;
// implementations return detached values so callers cannot mutate session state
// through shared maps, slices, pointers, or struct fields.
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
// implementations. Implementations must not retain ctx or caller-owned value
// references after an operation returns.
type Storage interface {
	Get(ctx context.Context, id string) (map[string]any, error)
	Set(ctx context.Context, id string, values map[string]any, ttl time.Duration) error
	Delete(ctx context.Context, id string) error
}

type memorySessionEntry struct {
	values    map[string]any
	expiresAt time.Time
}

// MemoryStorage is a process-local session store. Expired entries are swept
// opportunistically during writes, bounding stale-session growth without a
// background goroutine or an additional storage lifecycle contract.
type MemoryStorage struct {
	mu          sync.RWMutex
	entries     map[string]memorySessionEntry
	nextCleanup time.Time
}

func NewMemoryStorage() *MemoryStorage {
	now := time.Now()
	return &MemoryStorage{
		entries:     make(map[string]memorySessionEntry),
		nextCleanup: now.Add(memoryCleanupInterval),
	}
}

func (storage *MemoryStorage) Get(_ context.Context, id string) (map[string]any, error) {
	now := time.Now()
	storage.mu.RLock()
	entry, ok := storage.entries[id]
	if ok && !memoryEntryExpired(entry, now) {
		values := sessionvalue.CloneMap(entry.values)
		storage.mu.RUnlock()
		return values, nil
	}
	storage.mu.RUnlock()
	if !ok {
		return nil, ErrNotFound
	}

	storage.mu.Lock()
	entry, ok = storage.entries[id]
	if !ok {
		storage.mu.Unlock()
		return nil, ErrNotFound
	}
	if memoryEntryExpired(entry, time.Now()) {
		delete(storage.entries, id)
		storage.mu.Unlock()
		return nil, ErrNotFound
	}
	values := sessionvalue.CloneMap(entry.values)
	storage.mu.Unlock()
	return values, nil
}

func memoryEntryExpired(entry memorySessionEntry, now time.Time) bool {
	return !entry.expiresAt.IsZero() && now.After(entry.expiresAt)
}

func (storage *MemoryStorage) Set(_ context.Context, id string, values map[string]any, ttl time.Duration) error {
	normalized, err := sessionvalue.NormalizeMap(values)
	if err != nil {
		return err
	}
	now := time.Now()
	entry := memorySessionEntry{values: normalized}
	if ttl > 0 {
		entry.expiresAt = now.Add(ttl)
	}
	storage.mu.Lock()
	storage.entries[id] = entry
	storage.cleanupExpiredLocked(now)
	storage.mu.Unlock()
	return nil
}

func (storage *MemoryStorage) cleanupExpiredLocked(now time.Time) {
	if !storage.nextCleanup.IsZero() && now.Before(storage.nextCleanup) {
		return
	}
	for id, entry := range storage.entries {
		if memoryEntryExpired(entry, now) {
			delete(storage.entries, id)
		}
	}
	storage.nextCleanup = now.Add(memoryCleanupInterval)
}

func (storage *MemoryStorage) Delete(_ context.Context, id string) error {
	storage.mu.Lock()
	delete(storage.entries, id)
	storage.mu.Unlock()
	return nil
}

// CookieRevocationStore is the persistence boundary for signed-cookie
// revocation. CookieStorage passes only SHA-256 hex digests, never raw tokens or
// family identifiers. Implementations intended for multiple processes must use
// shared durable storage.
//
// RegisterFamilyToken must atomically reject a currently revoked family with
// ErrNotFound and update that family's latest token expiry. RevokeFamily must
// revoke through the greater of expiresAt and every expiry previously observed
// for the family, so Logout also covers tokens issued by requests already in
// flight.
type CookieRevocationStore interface {
	IsRevoked(ctx context.Context, tokenDigest, familyDigest string, now int64) (bool, error)
	RegisterFamilyToken(ctx context.Context, familyDigest string, expiresAt, now int64) error
	RevokeToken(ctx context.Context, tokenDigest string, expiresAt int64) error
	RevokeFamily(ctx context.Context, familyDigest string, expiresAt, now int64) error
}

// MemoryCookieRevocationStore is an explicit process-local implementation for
// tests and single-process applications. Its state is intentionally lost on
// process restart; authentication deployments that require logout/revocation
// across restart or multiple instances should provide a durable shared store.
type MemoryCookieRevocationStore struct {
	mu             sync.Mutex
	tokens         map[string]int64
	families       map[string]int64
	familyExpiries map[string]int64
}

func NewMemoryCookieRevocationStore() *MemoryCookieRevocationStore {
	return &MemoryCookieRevocationStore{
		tokens:         make(map[string]int64),
		families:       make(map[string]int64),
		familyExpiries: make(map[string]int64),
	}
}

func (store *MemoryCookieRevocationStore) IsRevoked(_ context.Context, tokenDigest, familyDigest string, now int64) (bool, error) {
	if store == nil {
		return false, errors.New("cookie revocation store is nil")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.cleanupLocked(now)
	return store.tokens[tokenDigest] > now || familyDigest != "" && store.families[familyDigest] > now, nil
}

func (store *MemoryCookieRevocationStore) RegisterFamilyToken(_ context.Context, familyDigest string, expiresAt, now int64) error {
	if store == nil {
		return errors.New("cookie revocation store is nil")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.cleanupLocked(now)
	if store.families[familyDigest] > now {
		return ErrNotFound
	}
	if expiresAt > store.familyExpiries[familyDigest] {
		store.familyExpiries[familyDigest] = expiresAt
	}
	return nil
}

func (store *MemoryCookieRevocationStore) RevokeToken(_ context.Context, tokenDigest string, expiresAt int64) error {
	if store == nil {
		return errors.New("cookie revocation store is nil")
	}
	store.mu.Lock()
	if expiresAt > store.tokens[tokenDigest] {
		store.tokens[tokenDigest] = expiresAt
	}
	store.mu.Unlock()
	return nil
}

func (store *MemoryCookieRevocationStore) RevokeFamily(_ context.Context, familyDigest string, expiresAt, now int64) error {
	if store == nil {
		return errors.New("cookie revocation store is nil")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.cleanupLocked(now)
	if familyExpiry := store.familyExpiries[familyDigest]; familyExpiry > expiresAt {
		expiresAt = familyExpiry
	}
	if expiresAt > store.families[familyDigest] {
		store.families[familyDigest] = expiresAt
	}
	return nil
}

func (store *MemoryCookieRevocationStore) cleanupLocked(now int64) {
	for key, expiry := range store.tokens {
		if expiry <= now {
			delete(store.tokens, key)
		}
	}
	for key, expiry := range store.families {
		if expiry <= now {
			delete(store.families, key)
		}
	}
	for key, expiry := range store.familyExpiries {
		if expiry <= now {
			delete(store.familyExpiries, key)
		}
	}
}

// CookieStorage stores signed, JSON-encoded session values in the cookie. It
// deliberately does not own revocation persistence; the application chooses a
// process-local or durable shared CookieRevocationStore explicitly.
type CookieStorage struct {
	secret      []byte
	revocations CookieRevocationStore
}

// NewCookieStorage creates signed-cookie session storage. The HMAC key must be
// at least 32 bytes of application-supplied secret material and a revocation
// store is mandatory so process-local revocation is never selected implicitly.
func NewCookieStorage(secret []byte, revocations CookieRevocationStore) (*CookieStorage, error) {
	if len(secret) < minimumCookieSecretBytes {
		return nil, fmt.Errorf("cookie session secret must be at least %d bytes", minimumCookieSecretBytes)
	}
	if isNilCookieRevocationStore(revocations) {
		return nil, errors.New("cookie session revocation store is required")
	}
	return &CookieStorage{secret: append([]byte(nil), secret...), revocations: revocations}, nil
}

func isNilCookieRevocationStore(store CookieRevocationStore) bool {
	if store == nil {
		return true
	}
	value := reflect.ValueOf(store)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

type cookiePayload struct {
	ExpiresAt int64          `json:"exp,omitempty"`
	Nonce     string         `json:"nonce,omitempty"`
	Family    string         `json:"family,omitempty"`
	Values    map[string]any `json:"values"`
}

func (storage *CookieStorage) Get(ctx context.Context, id string) (map[string]any, error) {
	payload, err := storage.decode(id)
	if err != nil {
		return nil, ErrNotFound
	}
	revoked, err := storage.revocations.IsRevoked(ctx, cookieDigest(id), cookieDigest(payload.Family), time.Now().Unix())
	if err != nil {
		return nil, fmt.Errorf("check cookie session revocation: %w", err)
	}
	if revoked {
		return nil, ErrNotFound
	}
	return sessionvalue.CloneMap(payload.Values), nil
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
	if err := sessionvalue.Decode(body, &payload); err != nil || (payload.ExpiresAt > 0 && time.Now().Unix() >= payload.ExpiresAt) {
		return cookiePayload{}, ErrNotFound
	}
	if payload.Values == nil {
		payload.Values = make(map[string]any)
	}
	return payload, nil
}

func (storage *CookieStorage) Set(context.Context, string, map[string]any, time.Duration) error {
	return errors.New("cookie session storage must be persisted through Session")
}

func (storage *CookieStorage) Delete(ctx context.Context, id string) error {
	payload, err := storage.decode(id)
	if err != nil {
		return nil
	}
	expiresAt := cookieExpiry(payload)
	if payload.Family == "" {
		return storage.revocations.RevokeToken(ctx, cookieDigest(id), expiresAt)
	}
	return storage.revocations.RevokeFamily(ctx, cookieDigest(payload.Family), expiresAt, time.Now().Unix())
}

// RevokeToken invalidates one signed cookie while leaving newer tokens in its
// session family usable. It is used during token rotation.
func (storage *CookieStorage) RevokeToken(ctx context.Context, id string) error {
	payload, err := storage.decode(id)
	if err != nil {
		return nil
	}
	return storage.revocations.RevokeToken(ctx, cookieDigest(id), cookieExpiry(payload))
}

// Encode creates a signed cookie value for SessionManager.
func (storage *CookieStorage) Encode(ctx context.Context, values map[string]any, ttl time.Duration) (string, error) {
	return storage.EncodeWithFamily(ctx, values, ttl, "")
}

// EncodeWithFamily creates a signed cookie value in family. An empty family
// starts a new independent session family.
func (storage *CookieStorage) EncodeWithFamily(ctx context.Context, values map[string]any, ttl time.Duration, family string) (string, error) {
	if storage == nil || len(storage.secret) == 0 || isNilCookieRevocationStore(storage.revocations) {
		return "", errors.New("cookie session storage is not configured")
	}
	normalized, err := sessionvalue.NormalizeMap(values)
	if err != nil {
		return "", err
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
	payload := cookiePayload{Nonce: nonce, Family: family, Values: normalized}
	if ttl > 0 {
		payload.ExpiresAt = time.Now().Add(ttl).Unix()
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	body = append(body, storage.sign(body)...)
	if err := storage.revocations.RegisterFamilyToken(
		ctx,
		cookieDigest(family),
		cookieExpiry(payload),
		time.Now().Unix(),
	); err != nil {
		return "", fmt.Errorf("register cookie session family: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(body), nil
}

func cookieExpiry(payload cookiePayload) int64 {
	if payload.ExpiresAt > 0 {
		return payload.ExpiresAt
	}
	return int64(^uint64(0) >> 1)
}

func cookieDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

// Family returns the signed cookie's session family, or an empty string when
// the value is not a valid signed token.
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

// NewID returns a cryptographically random identifier suitable for sessions
// and signed-cookie nonces.
func NewID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate secure session identifier: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}

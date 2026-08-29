package session

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCookieRevocationStoreSharesLogoutAcrossStorageInstances(t *testing.T) {
	ctx := context.Background()
	secret := make([]byte, 32)
	store := NewMemoryCookieRevocationStore()
	first, err := NewCookieStorage(secret, store)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewCookieStorage(secret, store)
	if err != nil {
		t.Fatal(err)
	}
	token, err := first.Encode(ctx, map[string]any{"user": "lane"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := second.Get(ctx, token); err != nil {
		t.Fatalf("shared storage could not read token before logout: %v", err)
	}
	if err := first.Delete(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Get(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("logged-out token remained usable across storage instances: %v", err)
	}
}

func TestCookieTokenRotationRevokesOnlyPreviousToken(t *testing.T) {
	ctx := context.Background()
	storage, err := NewCookieStorage(make([]byte, 32), NewMemoryCookieRevocationStore())
	if err != nil {
		t.Fatal(err)
	}
	first, err := storage.Encode(ctx, map[string]any{"version": 1}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	family := storage.Family(first)
	second, err := storage.EncodeWithFamily(ctx, map[string]any{"version": 2}, time.Hour, family)
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.RevokeToken(ctx, first); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Get(ctx, first); !errors.Is(err, ErrNotFound) {
		t.Fatalf("previous token remained usable: %v", err)
	}
	values, err := storage.Get(ctx, second)
	if err != nil {
		t.Fatalf("replacement token was revoked with previous token: %v", err)
	}
	if values["version"].(interface{ String() string }).String() != "2" {
		t.Fatalf("replacement values = %#v", values)
	}
}

func TestCookieFamilyLogoutRevokesTokensAlreadyIssuedInFamily(t *testing.T) {
	ctx := context.Background()
	storage, err := NewCookieStorage(make([]byte, 32), NewMemoryCookieRevocationStore())
	if err != nil {
		t.Fatal(err)
	}
	first, err := storage.Encode(ctx, map[string]any{"version": 1}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	second, err := storage.EncodeWithFamily(ctx, map[string]any{"version": 2}, 2*time.Hour, storage.Family(first))
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.Delete(ctx, first); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{first, second} {
		if _, err := storage.Get(ctx, token); !errors.Is(err, ErrNotFound) {
			t.Fatalf("family token remained usable after logout: %v", err)
		}
	}
}

package session

import (
	"context"
	"testing"
	"time"
)

func TestMemoryStorageSweepsExpiredEntriesDuringWrites(t *testing.T) {
	storage := NewMemoryStorage()
	now := time.Now()

	storage.mu.Lock()
	storage.entries["expired"] = memorySessionEntry{
		values:    map[string]any{"value": "stale"},
		expiresAt: now.Add(-time.Minute),
	}
	storage.entries["active"] = memorySessionEntry{
		values:    map[string]any{"value": "live"},
		expiresAt: now.Add(time.Hour),
	}
	storage.nextCleanup = now.Add(-time.Second)
	storage.mu.Unlock()

	if err := storage.Set(context.Background(), "new", map[string]any{"value": "fresh"}, time.Hour); err != nil {
		t.Fatal(err)
	}

	storage.mu.RLock()
	_, expiredExists := storage.entries["expired"]
	_, activeExists := storage.entries["active"]
	_, newExists := storage.entries["new"]
	storage.mu.RUnlock()

	if expiredExists {
		t.Fatal("expired session entry was not swept")
	}
	if !activeExists || !newExists {
		t.Fatalf("active entries were removed: active=%v new=%v", activeExists, newExists)
	}
}

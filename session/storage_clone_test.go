package session

import (
	"context"
	"testing"
	"time"
)

func TestMemoryStorageDeepClonesNestedValues(t *testing.T) {
	storage := NewMemoryStorage()
	original := map[string]any{
		"roles": []string{"user"},
		"profile": map[string]any{"name": "lane"},
	}
	if err := storage.Set(context.Background(), "id", original, time.Hour); err != nil {
		t.Fatal(err)
	}
	original["roles"].([]string)[0] = "admin"
	original["profile"].(map[string]any)["name"] = "changed"

	stored, err := storage.Get(context.Background(), "id")
	if err != nil {
		t.Fatal(err)
	}
	if stored["roles"].([]string)[0] != "user" || stored["profile"].(map[string]any)["name"] != "lane" {
		t.Fatalf("storage retained caller aliases: %#v", stored)
	}

	stored["roles"].([]string)[0] = "mutated"
	storedAgain, err := storage.Get(context.Background(), "id")
	if err != nil {
		t.Fatal(err)
	}
	if storedAgain["roles"].([]string)[0] != "user" {
		t.Fatalf("Get exposed internal nested state: %#v", storedAgain)
	}
}

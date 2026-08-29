package session

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestMemoryStorageCanonicalizesAndDetachesNestedValues(t *testing.T) {
	storage := NewMemoryStorage()
	original := map[string]any{
		"roles": []string{"user"},
		"profile": map[string]any{"name": "lane"},
		"count": int64(9007199254740993),
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
	roles, ok := stored["roles"].([]any)
	if !ok || len(roles) != 1 || roles[0] != "user" {
		t.Fatalf("roles = %#v", stored["roles"])
	}
	profile, ok := stored["profile"].(map[string]any)
	if !ok || profile["name"] != "lane" {
		t.Fatalf("profile = %#v", stored["profile"])
	}
	count, ok := stored["count"].(json.Number)
	if !ok || count.String() != "9007199254740993" {
		t.Fatalf("count = %#v", stored["count"])
	}

	roles[0] = "mutated"
	profile["name"] = "mutated"
	storedAgain, err := storage.Get(context.Background(), "id")
	if err != nil {
		t.Fatal(err)
	}
	if storedAgain["roles"].([]any)[0] != "user" || storedAgain["profile"].(map[string]any)["name"] != "lane" {
		t.Fatalf("Get exposed internal nested state: %#v", storedAgain)
	}
}

func TestMemoryStorageRejectsNonJSONValues(t *testing.T) {
	storage := NewMemoryStorage()
	if err := storage.Set(context.Background(), "id", map[string]any{"bad": func() {}}, time.Hour); err == nil {
		t.Fatal("non-JSON session value was accepted")
	}
}

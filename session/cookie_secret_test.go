package session

import "testing"

func TestNewCookieStorageRequiresStrongSecretAndRevocationStore(t *testing.T) {
	store := NewMemoryCookieRevocationStore()
	for _, size := range []int{0, 1, 16, 31} {
		if _, err := NewCookieStorage(make([]byte, size), store); err == nil {
			t.Fatalf("accepted cookie secret with %d bytes", size)
		}
	}
	if _, err := NewCookieStorage(make([]byte, 32), nil); err == nil {
		t.Fatal("accepted nil cookie revocation store")
	}
	var typedNil *MemoryCookieRevocationStore
	if _, err := NewCookieStorage(make([]byte, 32), typedNil); err == nil {
		t.Fatal("accepted typed-nil cookie revocation store")
	}
	storage, err := NewCookieStorage(make([]byte, 32), store)
	if err != nil {
		t.Fatal(err)
	}
	if storage == nil {
		t.Fatal("strong cookie secret and revocation store returned nil storage")
	}
}

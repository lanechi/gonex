package session

import "testing"

func TestNewCookieStorageRequiresStrongSecret(t *testing.T) {
	for _, size := range []int{0, 1, 16, 31} {
		if _, err := NewCookieStorage(make([]byte, size)); err == nil {
			t.Fatalf("accepted cookie secret with %d bytes", size)
		}
	}
	storage, err := NewCookieStorage(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	if storage == nil {
		t.Fatal("strong cookie secret returned nil storage")
	}
}

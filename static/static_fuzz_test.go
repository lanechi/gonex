package static

import (
	"io/fs"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

func FuzzSafeFilePath(f *testing.F) {
	for _, seed := range []string{
		"index.html",
		"assets/app.js",
		"../secret.txt",
		"%2e%2e/secret.txt",
		"%252e%252e%252fsecret.txt",
		"/absolute.js",
		`..\\secret.js`,
		"a/./b.js",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		got, ok := safeFilePath(value)
		if !ok {
			return
		}
		if got == "" || strings.Contains(got, `\\`) || path.IsAbs(got) || filepath.IsAbs(got) || path.Clean(got) != got || !fs.ValidPath(got) {
			t.Fatalf("unsafe path accepted: input=%q decoded=%q", value, got)
		}
	})
}

func FuzzValidMountPath(f *testing.F) {
	for _, seed := range []string{"/", "/assets", "assets", "/a/../b", "/a%2fb", `/a\\b`, "/a?b", "/a#b"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		if !validMountPath(value) {
			return
		}
		if !strings.HasPrefix(value, "/") || strings.ContainsAny(value, `\\?#`) || path.Clean(value) != value {
			t.Fatalf("unsafe mount path accepted: %q", value)
		}
	})
}

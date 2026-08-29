package config

import "testing"

func TestEnvironmentLookupDoesNotUseCompactFallback(t *testing.T) {
	t.Setenv("FOOBAR", "wrong")
	configuration := newForRoot(t.TempDir())
	if got := configuration.GetString("foo.bar"); got != "" {
		t.Fatalf("compact environment collision resolved unexpectedly: %q", got)
	}
	t.Setenv("FOO_BAR", "right")
	if got := configuration.GetString("foo.bar"); got != "right" {
		t.Fatalf("exact environment key not resolved: %q", got)
	}
}

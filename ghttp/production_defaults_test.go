package ghttp

import "testing"

func TestNormalizeGinModeDefaultsToRelease(t *testing.T) {
	mode, err := normalizeGinMode("")
	if err != nil {
		t.Fatal(err)
	}
	if mode != ReleaseMode {
		t.Fatalf("default mode = %q, want %q", mode, ReleaseMode)
	}
}

func TestInvalidGinModeFallsBackToRelease(t *testing.T) {
	server := newServerDefaults(nil)
	server.mode = "invalid"
	server.applyModeConfig()
	if server.mode != ReleaseMode {
		t.Fatalf("fallback mode = %q, want %q", server.mode, ReleaseMode)
	}
	if server.IsDebug() {
		t.Fatal("invalid mode fallback unexpectedly enabled debug mode")
	}
	if server.Err() == nil {
		t.Fatal("invalid mode did not preserve initialization error")
	}
}

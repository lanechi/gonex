package ghttp

import (
	"strings"
	"testing"

	gonexconfig "github.com/lanechi/gonex/config"
	"github.com/lanechi/gonex/session"
)

func TestCookieSessionConfigRequiresExplicitRevocationMode(t *testing.T) {
	configuration := gonexconfig.New()
	configuration.Set("session.storage.type", "cookie")
	configuration.Set("session.storage.secret", "cookie-config-secret-at-least-thirty-two-bytes")

	server := NewServer(WithConfig(configuration))
	if err := server.Err(); err == nil || !strings.Contains(err.Error(), "session.storage.revocation") {
		t.Fatalf("cookie config without explicit revocation error = %v", err)
	}
}

func TestCookieSessionConfigAllowsExplicitMemoryRevocation(t *testing.T) {
	configuration := gonexconfig.New()
	configuration.Set("session.storage.type", "cookie")
	configuration.Set("session.storage.secret", "cookie-config-secret-at-least-thirty-two-bytes")
	configuration.Set("session.storage.revocation", "memory")

	server := NewServer(WithConfig(configuration))
	if err := server.Err(); err != nil {
		t.Fatalf("explicit memory revocation config failed: %v", err)
	}
	if server.sessionManager == nil {
		t.Fatal("cookie session manager was not configured")
	}
	if _, ok := server.sessionManager.storage.(*session.CookieStorage); !ok {
		t.Fatalf("configured storage = %T, want *session.CookieStorage", server.sessionManager.storage)
	}
}

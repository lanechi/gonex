package db

import (
	"net/url"
	"testing"
)

func TestPostgresDialector(t *testing.T) {
	dialector, err := postgresDialector(postgresSettings{
		DSN: "host=127.0.0.1 user=postgres dbname=app sslmode=disable",
	})
	if err != nil || dialector == nil {
		t.Fatalf("postgresDialector() = %v, %v", dialector, err)
	}
}

func TestPostgresDSNEscapesConnectionFields(t *testing.T) {
	dsn := postgresDSN(postgresSettings{
		Host: "2001:db8::1", Port: 5433, User: "user@example.com", Password: "p@ ss",
		Name: "app db", SSLMode: "require", TimeZone: "America/Los_Angeles",
	})
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	password, _ := parsed.User.Password()
	if parsed.Scheme != "postgres" || parsed.Hostname() != "2001:db8::1" || parsed.Port() != "5433" ||
		parsed.User.Username() != "user@example.com" || password != "p@ ss" || parsed.Path != "/app db" ||
		parsed.Query().Get("sslmode") != "require" || parsed.Query().Get("TimeZone") != "America/Los_Angeles" {
		t.Fatalf("unexpected PostgreSQL URL: %s", dsn)
	}
}

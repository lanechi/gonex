package database

import (
	"net/url"
	"testing"
)

func TestDialectorOnlyAcceptsPostgreSQL(t *testing.T) {
	for _, driver := range []string{"postgres", "postgresql"} {
		t.Run(driver, func(t *testing.T) {
			dialector, err := dialector(settings{
				Driver: driver,
				DSN:    "host=127.0.0.1 user=postgres dbname=app sslmode=disable",
			})
			if err != nil || dialector == nil {
				t.Fatalf("dialector(%q) = %v, %v", driver, dialector, err)
			}
		})
	}

	for _, driver := range []string{"", "unsupported"} {
		t.Run(driver, func(t *testing.T) {
			if _, err := dialector(settings{Driver: driver, DSN: "ignored"}); err == nil {
				t.Fatalf("dialector(%q) returned no error", driver)
			}
		})
	}
}

func TestLoadSettingsDefaultsToPostgreSQL(t *testing.T) {
	settings := loadSettings(testConfig{})
	if settings.Driver != "postgres" {
		t.Fatalf("default driver = %q, want postgres", settings.Driver)
	}
}

func TestPostgresDSNEscapesConnectionFields(t *testing.T) {
	dsn := postgresDSN(settings{
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

type testConfig map[string]string

func (configuration testConfig) Get(key string) any { return configuration[key] }

func (configuration testConfig) GetString(key string) string { return configuration[key] }

func (configuration testConfig) GetInt(string) int { return 0 }

func (configuration testConfig) GetBool(string) bool { return false }

func (configuration testConfig) Unmarshal(any) error { return nil }

package dao

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultModelEnv = ".env"

// DatabaseConfig is the database configuration read from the project's
// DATABASE_* environment variables. DSN is preferred; the remaining fields
// make common development configurations convenient to express in .env.
type DatabaseConfig struct {
	Driver   string
	DSN      string
	URL      string
	Host     string
	Port     int
	Username string
	User     string
	Password string
	Database string
	Name     string
	SSLMode  string
	TimeZone string
}

// LoadDatabaseEnv loads the database configuration used by the project from
// the project .env file and the process environment. System environment
// variables take precedence over .env values. Database settings intentionally
// do not come from config.yaml; both the application and gx dao use the same
// DATABASE_* environment namespace.
func LoadDatabaseEnv(projectRoot string) (DatabaseConfig, error) {
	configuration := DatabaseConfig{}
	dotEnvPath := filepath.Join(projectRoot, defaultModelEnv)
	values, err := readDotEnv(dotEnvPath)
	if err != nil {
		return DatabaseConfig{}, err
	}
	if err := applyDatabaseEnvironment(&configuration, values); err != nil {
		return DatabaseConfig{}, err
	}

	configuration.Driver = strings.ToLower(strings.TrimSpace(configuration.Driver))
	if configuration.Driver == "" {
		configuration.Driver = inferDatabaseDriver(configuration.DSN, configuration.URL)
	}
	if configuration.Driver == "" {
		configuration.Driver = "sqlite"
	}
	if configuration.DSN == "" && configuration.URL == "" && configuration.Driver == "sqlite" {
		configuration.DSN = "data/app.db"
	}
	return configuration, nil
}

func readDotEnv(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read environment file %s: %w", path, err)
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		separator := strings.IndexByte(line, '=')
		if separator <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:separator])
		if !validEnvName(key) {
			continue
		}
		values[key] = parseDotEnvValue(line[separator+1:])
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read environment file %s: %w", path, err)
	}
	return values, nil
}

func validEnvName(value string) bool {
	if value == "" {
		return false
	}
	for index, character := range value {
		if (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '_' {
			if index == 0 && character >= '0' && character <= '9' {
				return false
			}
			continue
		}
		return false
	}
	return true
}

func parseDotEnvValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
		return value[1 : len(value)-1]
	}
	if len(value) >= 2 && value[0] == '"' {
		if parsed, err := strconv.Unquote(value); err == nil {
			return parsed
		}
	}
	if comment := strings.Index(value, " #"); comment >= 0 {
		value = value[:comment]
	}
	return strings.TrimSpace(value)
}

func databaseEnvValue(dotEnv map[string]string, name string) (string, bool) {
	if value, ok := os.LookupEnv(name); ok {
		return strings.TrimSpace(value), true
	}
	if value, ok := dotEnv[name]; ok {
		return strings.TrimSpace(value), true
	}
	return "", false
}

func applyDatabaseEnvironment(configuration *DatabaseConfig, dotEnv map[string]string) error {
	if configuration == nil {
		return nil
	}
	stringValues := []struct {
		name  string
		value *string
	}{
		{"DATABASE_DRIVER", &configuration.Driver},
		{"DATABASE_DSN", &configuration.DSN},
		{"DATABASE_URL", &configuration.URL},
		{"DATABASE_HOST", &configuration.Host},
		{"DATABASE_USERNAME", &configuration.Username},
		{"DATABASE_USER", &configuration.User},
		{"DATABASE_PASSWORD", &configuration.Password},
		{"DATABASE_DATABASE", &configuration.Database},
		{"DATABASE_NAME", &configuration.Name},
		{"DATABASE_SSLMODE", &configuration.SSLMode},
		{"DATABASE_TIMEZONE", &configuration.TimeZone},
	}
	for _, item := range stringValues {
		if value, ok := databaseEnvValue(dotEnv, item.name); ok {
			*item.value = value
		}
	}
	if value, ok := databaseEnvValue(dotEnv, "DATABASE_PORT"); ok {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 65535 {
			return fmt.Errorf("invalid DATABASE_PORT %q", value)
		}
		configuration.Port = parsed
	}
	return nil
}

func inferDatabaseDriver(dsn, url string) string {
	value := strings.ToLower(strings.TrimSpace(dsn))
	if value == "" {
		value = strings.ToLower(strings.TrimSpace(url))
	}
	switch {
	case value == ":memory:",
		strings.HasSuffix(value, ".db"),
		strings.HasSuffix(value, ".sqlite"),
		strings.HasSuffix(value, ".sqlite3"):
		return "sqlite"
	case strings.HasPrefix(value, "postgres://"),
		strings.HasPrefix(value, "postgresql://"),
		strings.Contains(value, " sslmode="):
		return "postgres"
	case strings.HasPrefix(value, "sqlserver://"):
		return "sqlserver"
	case strings.Contains(value, "@tcp("):
		return "mysql"
	default:
		return ""
	}
}

func resolveDatabasePaths(projectRoot string, configuration *DatabaseConfig) {
	if configuration == nil {
		return
	}
	driver := strings.ToLower(strings.TrimSpace(configuration.Driver))
	if (driver == "sqlite" || driver == "sqlite3") && configuration.DSN != "" && configuration.DSN != ":memory:" &&
		!strings.HasPrefix(configuration.DSN, "file:") &&
		!filepath.IsAbs(configuration.DSN) {
		configuration.DSN = filepath.Join(projectRoot, configuration.DSN)
	}
}

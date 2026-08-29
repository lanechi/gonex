package test

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lanechi/gonex/gx/internal/gen"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestDatabaseEnvironmentParsing(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".env"), "DATABASE_DRIVER=postgres\nDATABASE_DSN=\"host=db.example password=pa#ss\"\nDATABASE_PORT=5432\n")
	clearDatabaseEnvironment(t)
	configuration, err := gen.LoadDatabaseEnv(root)
	if err != nil {
		t.Fatal(err)
	}
	if configuration.Driver != "postgres" || configuration.DSN != "host=db.example password=pa#ss" || configuration.Port != 5432 {
		t.Fatalf("unexpected configuration: %#v", configuration)
	}

	writeFile(t, filepath.Join(root, ".env"), "DATABASE_PORT=0\n")
	if _, err := gen.LoadDatabaseEnv(root); err == nil {
		t.Fatal("invalid database port was accepted")
	}
}

func TestDatabaseModelGeneration(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres", "mysql"} {
		driver := driver
		t.Run(driver, func(t *testing.T) {
			configuration, ok := integrationDatabaseConfig(t, driver)
			if !ok {
				t.Skipf("no %s integration database configured", driver)
			}
			cleanupDatabase := func() {}
			if driver == "mysql" {
				var err error
				configuration, cleanupDatabase, err = prepareMySQLDatabase(configuration)
				if err != nil {
					t.Fatalf("prepare MySQL test database: %v", err)
				}
				t.Cleanup(cleanupDatabase)
			}

			root := t.TempDir()
			project := newProject(t, root)
			if driver == "sqlite" {
				configuration.DSN = filepath.Join(root, "integration.db")
			}
			clearDatabaseEnvironment(t)
			writeDatabaseEnv(t, root, configuration)

			database, err := openTestDatabase(configuration)
			if err != nil {
				t.Fatalf("open %s database: %v", driver, err)
			}
			sqlDatabase, err := database.DB()
			if err != nil {
				t.Fatal(err)
			}
			defer sqlDatabase.Close()

			table := fmt.Sprintf("gx_test_models_%d", time.Now().UnixNano())
			if err := createTestTable(database, driver, table); err != nil {
				t.Fatalf("create %s table: %v", driver, err)
			}
			defer func() { _ = database.Exec("DROP TABLE IF EXISTS " + table).Error }()

			result, err := gen.GenerateModels(project, gen.ModelOptions{Tables: table})
			if err != nil {
				t.Fatalf("generate %s models: %v", driver, err)
			}
			if len(result.Changes) == 0 {
				t.Fatal("model generation returned no changes")
			}
			assertGeneratedModelFiles(t, root)
		})
	}
}

func TestDatabaseGenerationFailureDoesNotPublishExistingOutput(t *testing.T) {
	root := t.TempDir()
	project := newProject(t, root)
	clearDatabaseEnvironment(t)
	writeDatabaseEnv(t, root, gen.DatabaseConfig{Driver: "sqlite", DSN: filepath.Join(root, "empty.db")})
	servicePath := filepath.Join(root, "internal/dao/keep.go")
	entityPath := filepath.Join(root, "internal/model/entity/keep.go")
	writeFile(t, servicePath, "package dao\n\nconst Keep = true\n")
	writeFile(t, entityPath, "package entity\n\nconst Keep = true\n")

	if _, err := gen.GenerateModels(project, gen.ModelOptions{}); err == nil {
		t.Fatal("empty database was accepted")
	}
	for _, path := range []string{servicePath, entityPath} {
		if content := string(mustRead(t, path)); !strings.Contains(content, "Keep = true") {
			t.Fatalf("generation failure replaced %s: %s", path, content)
		}
	}
}

func integrationDatabaseConfig(t *testing.T, driver string) (gen.DatabaseConfig, bool) {
	t.Helper()
	switch driver {
	case "sqlite":
		return gen.DatabaseConfig{Driver: driver, DSN: ":memory:"}, true
	case "postgres":
		if dsn := strings.TrimSpace(os.Getenv("GX_TEST_POSTGRES_DSN")); dsn != "" {
			return gen.DatabaseConfig{Driver: driver, DSN: dsn}, true
		}
		if os.Getenv("GX_TEST_POSTGRES") != "1" {
			return gen.DatabaseConfig{}, false
		}
		configuration, err := gen.LoadDatabaseEnv(filepath.Join(repositoryRoot(), "gx"))
		if err != nil {
			t.Fatalf("read PostgreSQL configuration: %v", err)
		}
		if strings.ToLower(configuration.Driver) != "postgres" && strings.ToLower(configuration.Driver) != "postgresql" && strings.ToLower(configuration.Driver) != "pgsql" {
			return gen.DatabaseConfig{}, false
		}
		configuration.Driver = driver
		return configuration, true
	case "mysql":
		if dsn := strings.TrimSpace(os.Getenv("GX_TEST_MYSQL_DSN")); dsn != "" {
			return gen.DatabaseConfig{Driver: driver, DSN: dsn}, true
		}
		password, ok := os.LookupEnv("GX_TEST_MYSQL_PASSWORD")
		if !ok {
			return gen.DatabaseConfig{}, false
		}
		return gen.DatabaseConfig{
			Driver: driver, Host: envOrDefault("GX_TEST_MYSQL_HOST", "localhost"),
			Port: parsePort(t, "GX_TEST_MYSQL_PORT", 3306),
			User: envOrDefault("GX_TEST_MYSQL_USER", "root"), Password: password,
			Database: strings.TrimSpace(os.Getenv("GX_TEST_MYSQL_DATABASE")),
		}, true
	default:
		return gen.DatabaseConfig{}, false
	}
}

func openTestDatabase(configuration gen.DatabaseConfig) (*gorm.DB, error) {
	dsn := strings.TrimSpace(configuration.DSN)
	if dsn == "" {
		dsn = strings.TrimSpace(configuration.URL)
	}
	switch strings.ToLower(configuration.Driver) {
	case "sqlite", "sqlite3":
		return gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Discard})
	case "postgres", "postgresql", "pgsql":
		if dsn == "" {
			port := configuration.Port
			if port == 0 {
				port = 5432
			}
			dsn = fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=disable", configuration.Host, firstNonEmpty(configuration.Username, configuration.User), configuration.Password, firstNonEmpty(configuration.Database, configuration.Name), port)
		}
		return gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	case "mysql", "mariadb", "tidb":
		if dsn == "" {
			port := configuration.Port
			if port == 0 {
				port = 3306
			}
			dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local", firstNonEmpty(configuration.Username, configuration.User), configuration.Password, configuration.Host, port, firstNonEmpty(configuration.Database, configuration.Name))
		}
		return gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Discard})
	default:
		return nil, fmt.Errorf("unsupported test database driver %q", configuration.Driver)
	}
}

func prepareMySQLDatabase(configuration gen.DatabaseConfig) (gen.DatabaseConfig, func(), error) {
	if configuration.Database != "" || configuration.DSN != "" {
		return configuration, func() {}, nil
	}
	port := configuration.Port
	if port == 0 {
		port = 3306
	}
	server, err := openTestDatabase(gen.DatabaseConfig{Driver: "mysql", Host: configuration.Host, Port: port, User: configuration.User, Password: configuration.Password, Database: ""})
	if err != nil {
		return gen.DatabaseConfig{}, nil, err
	}
	databaseName := fmt.Sprintf("gx_test_%d", time.Now().UnixNano())
	if err := server.Exec("CREATE DATABASE `" + databaseName + "`").Error; err != nil {
		if sqlDatabase, dbErr := server.DB(); dbErr == nil {
			_ = sqlDatabase.Close()
		}
		return gen.DatabaseConfig{}, nil, err
	}
	configuration.Database = databaseName
	return configuration, func() {
		_ = server.Exec("DROP DATABASE IF EXISTS `" + databaseName + "`").Error
		if sqlDatabase, dbErr := server.DB(); dbErr == nil {
			_ = sqlDatabase.Close()
		}
	}, nil
}

func createTestTable(database *gorm.DB, driver, table string) error {
	statement := `CREATE TABLE ` + table + ` (id BIGINT PRIMARY KEY, name VARCHAR(255) NOT NULL, active BOOLEAN NOT NULL)`
	if driver == "sqlite" {
		statement = `CREATE TABLE ` + table + ` (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, active BOOLEAN NOT NULL)`
	}
	return database.Exec(statement).Error
}

func newProject(t *testing.T, root string) gen.Project {
	t.Helper()
	module := string(mustRead(t, filepath.Join(repositoryRoot(), "gx", "go.mod")))
	module = strings.Replace(module, "module github.com/lanechi/gonex/gx", "module example.com/gx-database-test", 1)
	writeFile(t, filepath.Join(root, "go.mod"), module)
	if sum, err := os.ReadFile(filepath.Join(repositoryRoot(), "gx", "go.sum")); err == nil {
		writeFile(t, filepath.Join(root, "go.sum"), string(sum))
	}
	return gen.Project{Root: root, WorkingDir: root, ModulePath: "example.com/gx-database-test"}
}

func writeDatabaseEnv(t *testing.T, root string, configuration gen.DatabaseConfig) {
	t.Helper()
	lines := []string{"DATABASE_DRIVER=" + configuration.Driver}
	if configuration.DSN != "" {
		lines = append(lines, "DATABASE_DSN="+strconv.Quote(configuration.DSN))
	} else if configuration.URL != "" {
		lines = append(lines, "DATABASE_URL="+strconv.Quote(configuration.URL))
	} else {
		lines = append(lines,
			"DATABASE_HOST="+strconv.Quote(configuration.Host),
			"DATABASE_PORT="+strconv.Itoa(configuration.Port),
			"DATABASE_USER="+strconv.Quote(configuration.User),
			"DATABASE_PASSWORD="+strconv.Quote(configuration.Password),
			"DATABASE_NAME="+strconv.Quote(configuration.Database),
		)
	}
	writeFile(t, filepath.Join(root, ".env"), strings.Join(lines, "\n")+"\n")
}

func assertGeneratedModelFiles(t *testing.T, root string) {
	t.Helper()
	for _, relative := range []string{"internal/dao", "internal/model/entity"} {
		count := 0
		if err := filepath.Walk(filepath.Join(root, relative), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && filepath.Ext(path) == ".go" {
				count++
			}
			return nil
		}); err != nil {
			t.Fatalf("walk generated %s: %v", relative, err)
		}
		if count == 0 {
			t.Fatalf("no generated Go files under %s", relative)
		}
	}
}

func clearDatabaseEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{"DATABASE_DRIVER", "DATABASE_DSN", "DATABASE_URL", "DATABASE_HOST", "DATABASE_PORT", "DATABASE_USERNAME", "DATABASE_USER", "DATABASE_PASSWORD", "DATABASE_DATABASE", "DATABASE_NAME", "DATABASE_SSLMODE", "DATABASE_TIMEZONE"} {
		value, exists := os.LookupEnv(name)
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
		if exists {
			t.Cleanup(func() { _ = os.Setenv(name, value) })
		} else {
			t.Cleanup(func() { _ = os.Unsetenv(name) })
		}
	}
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func parsePort(t *testing.T, name string, fallback int) int {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		t.Fatalf("invalid %s=%q", name, value)
	}
	return port
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

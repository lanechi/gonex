package test

import (
	"fmt"
	"os"
	"os/exec"
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
			if driver == "postgres" {
				assertPostgresGeneratedArrayContract(t, root)
				writePostgresGeneratedRoundTripTest(t, root, table)
				runGeneratedProjectTests(t, root)
			}
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
	switch driver {
	case "sqlite":
		statement = `CREATE TABLE ` + table + ` (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, active BOOLEAN NOT NULL)`
	case "postgres":
		statement = `CREATE TABLE ` + table + ` (
			id BIGINT PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			active BOOLEAN NOT NULL,
			source_urls TEXT[],
			related_ids BIGINT[],
			flags BOOLEAN[],
			weights DOUBLE PRECISION[]
		)`
	}
	return database.Exec(statement).Error
}

func assertPostgresGeneratedArrayContract(t *testing.T, root string) {
	t.Helper()
	entityRoot := filepath.Join(root, "internal/model/entity")
	var generated strings.Builder
	if err := filepath.Walk(entityRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		generated.Write(content)
		generated.WriteByte('\n')
		return nil
	}); err != nil {
		t.Fatalf("read generated PostgreSQL entities: %v", err)
	}

	content := generated.String()
	for _, want := range []string{
		"[]string",
		"[]int64",
		"[]bool",
		"[]float64",
		"column:source_urls",
		"column:related_ids",
		"serializer:pgarray",
		"pgarray:_text",
		"pgarray:_int8",
		"pgarray:_bool",
		"pgarray:_float8",
		"type:text[]",
		"type:bigint[]",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("generated PostgreSQL entity output missing %q:\n%s", want, content)
		}
	}
	for _, forbidden := range []string{
		"*[]string",
		"*[]int64",
		"*[]bool",
		"*[]float64",
		"pgtype.Array[string]",
		"pgtype.FlatArray[string]",
		"pq.StringArray",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("generated PostgreSQL entity output contains forbidden type %q:\n%s", forbidden, content)
		}
	}
	if _, err := os.Stat(filepath.Join(entityRoot, "pgarray_serializer.gen.go")); err != nil {
		t.Fatalf("generated PostgreSQL array serializer missing: %v", err)
	}
}

func writePostgresGeneratedRoundTripTest(t *testing.T, root, table string) {
	t.Helper()
	path := filepath.Join(root, "internal/model/entity/pgarray_roundtrip_test.go")
	content := fmt.Sprintf(`package entity

import (
	"os"
	"reflect"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type pgArrayRoundTripRow struct {
	ID         int64     __TICK__gorm:"column:id;primaryKey"__TICK__
	Name       string    __TICK__gorm:"column:name"__TICK__
	Active     bool      __TICK__gorm:"column:active"__TICK__
	SourceURLs []string  __TICK__gorm:"column:source_urls;type:text[];serializer:pgarray;pgarray:_text"__TICK__
	RelatedIDs []int64   __TICK__gorm:"column:related_ids;type:bigint[];serializer:pgarray;pgarray:_int8"__TICK__
	Flags      []bool    __TICK__gorm:"column:flags;type:boolean[];serializer:pgarray;pgarray:_bool"__TICK__
	Weights    []float64 __TICK__gorm:"column:weights;type:double precision[];serializer:pgarray;pgarray:_float8"__TICK__
}

func TestGeneratedPostgresArraySerializerRoundTrip(t *testing.T) {
	dsn := os.Getenv("GX_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("GX_TEST_POSTGRES_DSN is not configured")
	}
	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	row := pgArrayRoundTripRow{
		ID: 1, Name: "arrays", Active: true,
		SourceURLs: []string{"https://a.example/a.jpg", "https://b.example/b.jpg"},
		RelatedIDs: []int64{11, 22},
		Flags: []bool{true, false},
		Weights: []float64{1.5, 2.5},
	}
	if err := database.Table(%q).Create(&row).Error; err != nil {
		t.Fatalf("create array row: %%v", err)
	}

	var got pgArrayRoundTripRow
	if err := database.Table(%q).Where("id = ?", row.ID).Take(&got).Error; err != nil {
		t.Fatalf("read array row: %%v", err)
	}
	if !reflect.DeepEqual(got.SourceURLs, row.SourceURLs) ||
		!reflect.DeepEqual(got.RelatedIDs, row.RelatedIDs) ||
		!reflect.DeepEqual(got.Flags, row.Flags) ||
		!reflect.DeepEqual(got.Weights, row.Weights) {
		t.Fatalf("array round trip mismatch: got=%%#v want=%%#v", got, row)
	}

	empty := []string{}
	if err := database.Table(%q).Where("id = ?", row.ID).
		Select("SourceURLs", "RelatedIDs").
		Updates(&pgArrayRoundTripRow{
			SourceURLs: empty,
			RelatedIDs: []int64{33, 44, 55},
		}).Error; err != nil {
		t.Fatalf("update arrays: %%v", err)
	}
	if err := database.Table(%q).Where("id = ?", row.ID).Take(&got).Error; err != nil {
		t.Fatalf("read updated arrays: %%v", err)
	}
	if got.SourceURLs == nil || len(got.SourceURLs) != 0 {
		t.Fatalf("empty array lost empty/non-nil semantics: %%#v", got.SourceURLs)
	}
	if !reflect.DeepEqual(got.RelatedIDs, []int64{33, 44, 55}) {
		t.Fatalf("updated related ids = %%#v", got.RelatedIDs)
	}

	got.SourceURLs = nil
	got.Flags = []bool{false, true, true}
	if err := database.Table(%q).Save(&got).Error; err != nil {
		t.Fatalf("save arrays: %%v", err)
	}
	if err := database.Table(%q).Where("id = ?", row.ID).Take(&got).Error; err != nil {
		t.Fatalf("read saved arrays: %%v", err)
	}
	if got.SourceURLs != nil {
		t.Fatalf("NULL array decoded as non-nil slice: %%#v", got.SourceURLs)
	}
	if !reflect.DeepEqual(got.Flags, []bool{false, true, true}) {
		t.Fatalf("saved flags = %%#v", got.Flags)
	}
}
`, table, table, table, table, table, table)
	content = strings.ReplaceAll(content, "__TICK__", "`")
	writeFile(t, path, content)
}

func runGeneratedProjectTests(t *testing.T, root string) {
	t.Helper()
	for _, arguments := range [][]string{
		{"mod", "tidy"},
		{"test", "./..."},
	} {
		command := exec.Command("go", arguments...)
		command.Dir = root
		command.Env = os.Environ()
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("generated project go %s failed: %v\n%s", strings.Join(arguments, " "), err, output)
		}
	}
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

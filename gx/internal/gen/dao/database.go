package dao

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	typemapping "github.com/lanechi/gonex/gx/internal/type_mapping"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/driver/sqlserver"
	"gorm.io/gen"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func isPostgresDriver(driver string) bool {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "postgres", "postgresql", "pgsql":
		return true
	default:
		return false
	}
}

func newModelGeneratorWithMapping(
	database *gorm.DB,
	outputRoot, modelRoot string,
	mapping typemapping.Mapping,
) *gen.Generator {
	generator := gen.NewGenerator(gen.Config{
		OutPath:             outputRoot,
		ModelPkgPath:        modelRoot,
		Mode:                gen.WithDefaultQuery | gen.WithQueryInterface,
		FieldNullable:       true,
		FieldCoverable:      true,
		FieldWithIndexTag:   true,
		FieldWithTypeTag:    true,
		FieldWithDefaultTag: true,
		Incremental:         false,
	})
	generator.UseDB(database)
	if len(mapping.TypeMap) > 0 {
		generator.WithDataTypeMap(mapping.TypeMap)
	}
	if len(mapping.Imports) > 0 {
		generator.WithImportPkgPath(mapping.Imports...)
	}
	generator.WithOpts(gen.FieldModify(stripNullableCollectionType))
	generator.SetLogger(modelGeneratorLogger{})
	return generator
}

func buildTypeMapping(
	database *gorm.DB,
	driver typemapping.DatabaseType,
	tableNames []string,
) (typemapping.Mapping, error) {
	introspected := make([]typemapping.TableColumns, 0, len(tableNames))
	for _, tableName := range tableNames {
		columns, err := database.Migrator().ColumnTypes(tableName)
		if err != nil {
			return typemapping.Mapping{}, fmt.Errorf("read columns for table %s: %w", tableName, err)
		}
		introspected = append(introspected, typemapping.TableColumns{Table: tableName, Columns: columns})
	}
	mapping := typemapping.BuildDataTypeMap(driver, introspected)
	for _, warning := range mapping.Warnings {
		fmt.Fprintf(os.Stderr, "WARN %s\n", warning.String())
	}
	return mapping, nil
}

func stripNullableCollectionType(field gen.Field) gen.Field {
	if field == nil {
		return nil
	}
	if strings.HasPrefix(field.Type, "*[]") || field.Type == "*datatypes.JSON" {
		field.Type = strings.TrimPrefix(field.Type, "*")
	}
	return field
}

func generateModels(database *gorm.DB, outputRoot, modelRoot string, tables []string) error {
	tableNames := append([]string(nil), tables...)
	if len(tableNames) == 0 {
		var err error
		tableNames, err = database.Migrator().GetTables()
		if err != nil {
			return fmt.Errorf("list database tables: %w", err)
		}
	}
	mapping, err := buildTypeMapping(database, typemapping.DatabaseType(database.Dialector.Name()), tableNames)
	if err != nil {
		return err
	}
	generator := newModelGeneratorWithMapping(database, outputRoot, modelRoot, mapping)
	models := make([]interface{}, 0)
	if len(tables) == 0 {
		for _, model := range generator.GenerateAllTable() {
			if model != nil {
				models = append(models, model)
			}
		}
	} else {
		for _, table := range tables {
			model := generator.GenerateModel(table)
			if model != nil {
				models = append(models, model)
			}
		}
	}
	if len(models) == 0 {
		return fmt.Errorf("no database tables found for model generation")
	}
	generator.ApplyBasic(models...)
	return executeModelGenerator(generator)
}

type postgresTable struct {
	Schema string `gorm:"column:table_schema"`
	Name   string `gorm:"column:table_name"`
}

func generatePostgresModels(project Project, database *gorm.DB, requested, outputRoot, modelRoot string) error {
	tables, err := listPostgresTables(database)
	if err != nil {
		return err
	}
	selected := selectPostgresTables(tables, requested)
	if len(selected) == 0 {
		return fmt.Errorf("no database tables found for model generation")
	}
	qualifiedTables := make([]string, 0, len(selected))
	for _, table := range selected {
		qualifiedTables = append(qualifiedTables, qualifiedPostgresTable(table))
	}
	mapping, err := buildTypeMapping(database, typemapping.DatabasePostgres, qualifiedTables)
	if err != nil {
		return err
	}

	bySchema := make(map[string][]postgresTable)
	for _, table := range selected {
		bySchema[table.Schema] = append(bySchema[table.Schema], table)
	}
	allSchemas := make(map[string]struct{})
	for _, table := range tables {
		allSchemas[table.Schema] = struct{}{}
	}
	schemas := make([]string, 0, len(bySchema))
	for schemaName := range bySchema {
		schemas = append(schemas, schemaName)
	}
	sort.Strings(schemas)

	// Preserve the original flat layout for a database that only exposes one
	// schema. Multiple PostgreSQL schemas use schema subdirectories.
	if len(allSchemas) == 1 {
		generator := newModelGeneratorWithMapping(database, outputRoot, modelRoot, mapping)
		generator.WithFileNameStrategy(stripPostgresSchemaFromFileName)
		models := make([]interface{}, 0, len(selected))
		for _, table := range selected {
			model := generator.GenerateModelAs(qualifiedPostgresTable(table), postgresModelName(table.Name))
			if model != nil {
				models = append(models, model)
			}
		}
		if len(models) == 0 {
			return fmt.Errorf("no database tables found for model generation")
		}
		generator.ApplyBasic(models...)
		return executeModelGenerator(generator)
	}

	for _, schemaName := range schemas {
		if err := generatePostgresSchema(project, database, schemaName, bySchema[schemaName], outputRoot, modelRoot, mapping); err != nil {
			return err
		}
	}
	return nil
}

func listPostgresTables(database *gorm.DB) ([]postgresTable, error) {
	var tables []postgresTable
	err := database.Raw(`
		SELECT table_schema, table_name
		FROM information_schema.tables
		WHERE table_type = 'BASE TABLE'
		  AND table_schema <> 'information_schema'
		  AND table_schema NOT LIKE 'pg_%'
		ORDER BY table_schema, table_name
	`).Scan(&tables).Error
	if err != nil {
		return nil, fmt.Errorf("list PostgreSQL schemas and tables: %w", err)
	}
	return tables, nil
}

func selectPostgresTables(all []postgresTable, requested string) []postgresTable {
	if strings.TrimSpace(requested) == "" {
		return all
	}

	requestedTables := splitTables(requested)
	selected := make([]postgresTable, 0, len(requestedTables))
	seen := make(map[string]struct{})
	for _, requestedTable := range requestedTables {
		requestedSchema, requestedName := splitPostgresTableReference(requestedTable)
		for _, table := range all {
			if requestedSchema != "" && requestedSchema != table.Schema {
				continue
			}
			if requestedName != table.Name {
				continue
			}
			key := table.Schema + "\x00" + table.Name
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			selected = append(selected, table)
		}
	}
	sort.Slice(selected, func(left, right int) bool {
		if selected[left].Schema == selected[right].Schema {
			return selected[left].Name < selected[right].Name
		}
		return selected[left].Schema < selected[right].Schema
	})
	return selected
}

func splitPostgresTableReference(value string) (schemaName, tableName string) {
	parts := strings.SplitN(value, ".", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return "", strings.TrimSpace(value)
}

func postgresModelName(tableName string) string {
	return schema.NamingStrategy{SingularTable: false}.SchemaName(tableName)
}

func qualifiedPostgresTable(table postgresTable) string {
	return table.Schema + "." + table.Name
}

func stripPostgresSchemaFromFileName(tableName string) string {
	if _, table := splitPostgresTableReference(tableName); table != "" {
		return table
	}
	return tableName
}

func generatePostgresSchema(
	project Project,
	database *gorm.DB,
	schemaName string,
	tables []postgresTable,
	outputRoot, modelRoot string,
	mapping typemapping.Mapping,
) error {
	if err := validateSchemaDirectory(schemaName); err != nil {
		return err
	}
	// Keep the staging directory inside the module so gorm/gen can resolve the
	// module import path, but do not use a dot-prefixed name because Go tooling
	// skips hidden directories when loading packages.
	stageRoot, err := os.MkdirTemp(project.Root, "gx-model-")
	if err != nil {
		return fmt.Errorf("create PostgreSQL schema staging directory: %w", err)
	}
	defer os.RemoveAll(stageRoot)

	stageDAO := filepath.Join(stageRoot, "dao")
	stageEntity := filepath.Join(stageRoot, "entity")
	generator := newModelGeneratorWithMapping(database, stageDAO, stageEntity, mapping)
	generator.WithFileNameStrategy(stripPostgresSchemaFromFileName)
	models := make([]interface{}, 0, len(tables))
	for _, table := range tables {
		model := generator.GenerateModelAs(qualifiedPostgresTable(table), postgresModelName(table.Name))
		if model != nil {
			models = append(models, model)
		}
	}
	if len(models) == 0 {
		return fmt.Errorf("no database tables found for PostgreSQL schema %q", schemaName)
	}
	generator.ApplyBasic(models...)
	if err := executeModelGenerator(generator); err != nil {
		return fmt.Errorf("generate PostgreSQL schema %q: %w", schemaName, err)
	}

	finalDAO := filepath.Join(outputRoot, schemaName)
	finalEntity := filepath.Join(modelRoot, schemaName)
	if err := moveGeneratedFiles(stageDAO, finalDAO); err != nil {
		return fmt.Errorf("move DAO files for PostgreSQL schema %q: %w", schemaName, err)
	}
	if err := moveGeneratedFiles(stageEntity, finalEntity); err != nil {
		return fmt.Errorf("move entity files for PostgreSQL schema %q: %w", schemaName, err)
	}
	oldEntityImport := projectImportPath(project, stageEntity)
	newEntityImport := projectImportPath(project, finalEntity)
	if err := rewriteGeneratedImport(finalDAO, oldEntityImport, newEntityImport); err != nil {
		return fmt.Errorf("rewrite DAO entity import for PostgreSQL schema %q: %w", schemaName, err)
	}
	return nil
}

func validateSchemaDirectory(schemaName string) error {
	if strings.TrimSpace(schemaName) == "" || schemaName == "." || schemaName == ".." ||
		strings.ContainsAny(schemaName, `/\\`) {
		return fmt.Errorf("invalid PostgreSQL schema name %q for generated directory", schemaName)
	}
	return nil
}

func splitTables(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' })
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

func openConfiguredDatabase(configuration DatabaseConfig) (*gorm.DB, error) {
	dialector, err := databaseDialector(configuration)
	if err != nil {
		return nil, err
	}
	database, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return nil, fmt.Errorf("open %s database: %w", configuration.Driver, err)
	}
	return database, nil
}

func databaseDialector(configuration DatabaseConfig) (gorm.Dialector, error) {
	driver := strings.ToLower(strings.TrimSpace(configuration.Driver))
	dsn := strings.TrimSpace(configuration.DSN)
	if dsn == "" {
		dsn = strings.TrimSpace(configuration.URL)
	}
	switch driver {
	case "sqlite", "sqlite3":
		if dsn == "" {
			dsn = "data/app.db"
		}
		if dsn != ":memory:" && !strings.HasPrefix(dsn, "file:") {
			if directory := filepath.Dir(dsn); directory != "." {
				if err := os.MkdirAll(directory, 0o755); err != nil {
					return nil, fmt.Errorf("create sqlite directory %s: %w", directory, err)
				}
			}
		}
		return sqlite.Open(dsn), nil
	case "mysql", "mariadb", "tidb":
		if dsn == "" {
			dsn = mysqlDSN(configuration)
		}
		if dsn == "" {
			return nil, fmt.Errorf("DATABASE_DSN or MySQL connection fields are required")
		}
		return mysql.Open(dsn), nil
	case "postgres", "postgresql", "pgsql":
		if dsn == "" {
			dsn = postgresDSN(configuration)
		}
		if dsn == "" {
			return nil, fmt.Errorf("DATABASE_DSN or PostgreSQL connection fields are required")
		}
		return postgres.Open(dsn), nil
	case "sqlserver", "mssql":
		if dsn == "" {
			dsn = sqlserverDSN(configuration)
		}
		if dsn == "" {
			return nil, fmt.Errorf("DATABASE_DSN or SQL Server connection fields are required")
		}
		return sqlserver.Open(dsn), nil
	default:
		return nil, fmt.Errorf(
			"unsupported database driver %q; supported drivers: mysql, postgres, sqlite, sqlserver",
			configuration.Driver,
		)
	}
}

func mysqlDSN(configuration DatabaseConfig) string {
	user := configuration.Username
	if user == "" {
		user = configuration.User
	}
	database := configuration.Database
	if database == "" {
		database = configuration.Name
	}
	if user == "" || configuration.Host == "" || database == "" {
		return ""
	}
	port := configuration.Port
	if port == 0 {
		port = 3306
	}
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		user,
		configuration.Password,
		configuration.Host,
		port,
		database,
	)
}

func postgresDSN(configuration DatabaseConfig) string {
	user := configuration.Username
	if user == "" {
		user = configuration.User
	}
	database := configuration.Database
	if database == "" {
		database = configuration.Name
	}
	if user == "" || configuration.Host == "" || database == "" {
		return ""
	}
	port := configuration.Port
	if port == 0 {
		port = 5432
	}
	sslMode := configuration.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}
	timeZone := configuration.TimeZone
	if timeZone == "" {
		timeZone = "UTC"
	}
	return fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%d sslmode=%s TimeZone=%s",
		configuration.Host,
		user,
		configuration.Password,
		database,
		port,
		sslMode,
		timeZone,
	)
}

func sqlserverDSN(configuration DatabaseConfig) string {
	if configuration.URL != "" {
		return configuration.URL
	}
	user := configuration.Username
	if user == "" {
		user = configuration.User
	}
	database := configuration.Database
	if database == "" {
		database = configuration.Name
	}
	if user == "" || configuration.Host == "" {
		return ""
	}
	port := configuration.Port
	if port == 0 {
		port = 1433
	}
	return "sqlserver://" + user + ":" + configuration.Password + "@" + configuration.Host + ":" + strconv.Itoa(
		port,
	) + "?database=" + database
}

type modelGeneratorLogger struct{}

func (modelGeneratorLogger) Println(...any) {}

func executeModelGenerator(generator *gen.Generator) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("gorm model generation failed: %v", recovered)
		}
	}()
	generator.Execute()
	return nil
}

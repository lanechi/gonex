package gen

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	typemapping "github.com/lanechi/gonex/gx/internal/type_mapping"
	"golang.org/x/mod/modfile"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/driver/sqlserver"
	"gorm.io/gen"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

const (
	defaultModelEnv     = ".env"
	defaultModelOutput  = "internal/dao"
	defaultEntityOutput = "internal/model/entity"
)

// ModelOptions controls database-to-GORM code generation. Output paths are
// project-relative and intentionally follow the init project layout.
type ModelOptions struct {
	Tables  string
	runTidy func(string) error
}

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

// GenerateModels connects to the configured database and delegates schema
// introspection and model/query rendering to gorm.io/gen.
func GenerateModels(project Project, options ModelOptions) (result Result, err error) {
	moduleBefore, err := snapshotModuleFiles(project.Root)
	if err != nil {
		return result, err
	}
	var outputTransaction *modelOutputTransaction
	moduleMutationStarted := false
	defer func() {
		if err == nil {
			if outputTransaction != nil {
				err = outputTransaction.Commit()
			}
			return
		}
		var rollbackErrors []error
		if outputTransaction != nil {
			if rollbackErr := outputTransaction.Rollback(); rollbackErr != nil {
				rollbackErrors = append(rollbackErrors, rollbackErr)
			}
		}
		if moduleMutationStarted {
			if restoreErr := restoreModuleFiles(project.Root, moduleBefore); restoreErr != nil {
				rollbackErrors = append(rollbackErrors, restoreErr)
			}
		}
		if len(rollbackErrors) > 0 {
			err = errors.Join(append([]error{err}, rollbackErrors...)...)
		}
	}()

	databaseConfig, err := LoadDatabaseEnv(project.Root)
	if err != nil {
		return result, err
	}
	resolveDatabasePaths(project.Root, &databaseConfig)
	database, err := openConfiguredDatabase(databaseConfig)
	if err != nil {
		return result, err
	}
	if sqlDatabase, dbErr := database.DB(); dbErr == nil {
		defer sqlDatabase.Close()
	}

	outputRoot := project.Resolve(defaultModelOutput)
	modelRoot := project.Resolve(defaultEntityOutput)
	before, err := snapshotFiles(project.Root, outputRoot, modelRoot)
	if err != nil {
		return result, err
	}
	stageRoot, err := os.MkdirTemp(project.Root, "gx-model-stage-")
	if err != nil {
		return result, fmt.Errorf("create model generation staging directory: %w", err)
	}
	defer os.RemoveAll(stageRoot)
	stageDAO := filepath.Join(stageRoot, "dao")
	stageEntity := filepath.Join(stageRoot, "entity")
	if isPostgresDriver(databaseConfig.Driver) {
		err = generatePostgresModels(project, database, options.Tables, stageDAO, stageEntity)
	} else {
		err = generateModels(database, stageDAO, stageEntity, splitTables(options.Tables))
	}
	if err != nil {
		return result, err
	}
	if err := removeGeneratedImportAliases(stageDAO, stageEntity); err != nil {
		return result, err
	}
	if err := rewriteGeneratedImport(stageDAO, projectImportPath(project, stageEntity), projectImportPath(project, modelRoot)); err != nil {
		return result, fmt.Errorf("rewrite staged DAO entity import: %w", err)
	}
	if err := addGeneratedModelHeaders(stageDAO, stageEntity); err != nil {
		return result, err
	}
	if err := sanitizeGeneratedStructTags(stageDAO, stageEntity); err != nil {
		return result, err
	}
	if err := validateGeneratedModelOutput(stageDAO, stageEntity); err != nil {
		return result, err
	}
	outputTransaction, err = replaceModelOutputDirectories(project, stageDAO, stageEntity)
	if err != nil {
		return result, err
	}
	after, err := snapshotFiles(project.Root, outputRoot, modelRoot)
	if err != nil {
		return result, err
	}
	addFileChanges(&result, before, after)
	moduleMutationStarted = true
	if err := ensureModelDependencies(project, &result); err != nil {
		return result, err
	}
	tidy := options.runTidy
	if tidy == nil {
		tidy = runGoModTidy
	}
	if err := tidy(project.Root); err != nil {
		return result, err
	}
	moduleAfter, err := snapshotModuleFiles(project.Root)
	if err != nil {
		return result, err
	}
	addModuleFileChanges(&result, moduleBefore, moduleAfter)
	return result, nil
}

func validateGeneratedModelOutput(roots ...string) error {
	for _, root := range roots {
		files := 0
		err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}
			files++
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, content, parser.ParseComments)
			if err != nil {
				return fmt.Errorf("parse generated model %s: %w", path, err)
			}
			if err := validateFileStructTags(file); err != nil {
				return fmt.Errorf("validate generated model %s: %w", path, err)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("validate generated model output %s: %w", root, err)
		}
		if files == 0 {
			return fmt.Errorf("generated model output %s contains no Go files", root)
		}
	}
	return nil
}

func sanitizeGeneratedStructTags(roots ...string) error {
	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}
			fileSet := token.NewFileSet()
			file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
			if err != nil {
				return fmt.Errorf("parse generated model %s: %w", path, err)
			}
			changed := false
			var tagErr error
			ast.Inspect(file, func(node ast.Node) bool {
				field, ok := node.(*ast.Field)
				if !ok || field.Tag == nil || tagErr != nil {
					return tagErr == nil
				}
				tag, err := strconv.Unquote(field.Tag.Value)
				if err != nil {
					tagErr = err
					return false
				}
				updated, sanitized, err := sanitizeGORMStructTag(tag)
				if err != nil {
					tagErr = err
					return false
				}
				if sanitized {
					field.Tag.Value = "`" + updated + "`"
					changed = true
				}
				return true
			})
			if tagErr != nil {
				return fmt.Errorf("sanitize generated struct tag in %s: %w", path, tagErr)
			}
			if !changed {
				return nil
			}
			var formatted bytes.Buffer
			if err := format.Node(&formatted, fileSet, file); err != nil {
				return fmt.Errorf("format generated model %s: %w", path, err)
			}
			formatted.WriteByte('\n')
			return os.WriteFile(path, formatted.Bytes(), info.Mode().Perm())
		})
		if err != nil {
			return fmt.Errorf("sanitize generated struct tags in %s: %w", root, err)
		}
	}
	return nil
}

func sanitizeGORMStructTag(tag string) (string, bool, error) {
	const prefix = `gorm:"`
	start := strings.Index(tag, prefix)
	if start < 0 || (start > 0 && tag[start-1] != ' ') {
		return tag, false, nil
	}
	valueStart := start + len(prefix)
	valueEnd := -1
	for index := valueStart; index < len(tag); index++ {
		if tag[index] != '"' || escapedAt(tag, index) {
			continue
		}
		if validStructTagBoundary(tag[index+1:]) {
			valueEnd = index
			break
		}
	}
	if valueEnd < 0 {
		return "", false, fmt.Errorf("gorm struct tag has no closing quote: %q", tag)
	}
	value := tag[valueStart:valueEnd]
	var builder strings.Builder
	changed := false
	for index := 0; index < len(value); index++ {
		if value[index] == '"' && !escapedAt(value, index) {
			builder.WriteByte('\\')
			changed = true
		}
		builder.WriteByte(value[index])
	}
	if !changed {
		return tag, false, nil
	}
	return tag[:valueStart] + builder.String() + tag[valueEnd:], true, nil
}

func escapedAt(value string, index int) bool {
	backslashes := 0
	for index--; index >= 0 && value[index] == '\\'; index-- {
		backslashes++
	}
	return backslashes%2 == 1
}

func validStructTagBoundary(remainder string) bool {
	if remainder == "" {
		return true
	}
	if remainder[0] != ' ' {
		return false
	}
	remainder = strings.TrimLeft(remainder, " ")
	if remainder == "" {
		return true
	}
	return validateStructTag(remainder) == nil
}

func validateFileStructTags(file *ast.File) error {
	var validationErr error
	ast.Inspect(file, func(node ast.Node) bool {
		field, ok := node.(*ast.Field)
		if !ok || field.Tag == nil || validationErr != nil {
			return validationErr == nil
		}
		tag, err := strconv.Unquote(field.Tag.Value)
		if err != nil {
			validationErr = err
			return false
		}
		validationErr = validateStructTag(tag)
		return validationErr == nil
	})
	return validationErr
}

func validateStructTag(tag string) error {
	structTag := reflect.StructTag(tag)
	remaining := tag
	for remaining != "" {
		remaining = strings.TrimLeft(remaining, " ")
		if remaining == "" {
			return nil
		}
		colon := strings.IndexByte(remaining, ':')
		if colon <= 0 || colon+1 >= len(remaining) || remaining[colon+1] != '"' {
			return fmt.Errorf("invalid struct tag %q", tag)
		}
		key := remaining[:colon]
		for _, character := range key {
			if character <= ' ' || character == '"' || character == ':' || character == 0x7f {
				return fmt.Errorf("invalid struct tag key %q", key)
			}
		}
		quoted := remaining[colon+1:]
		end := 1
		for end < len(quoted) {
			if quoted[end] == '"' && !escapedAt(quoted, end) {
				break
			}
			end++
		}
		if end >= len(quoted) {
			return fmt.Errorf("unterminated struct tag value for %q", key)
		}
		if _, err := strconv.Unquote(quoted[:end+1]); err != nil {
			return fmt.Errorf("invalid struct tag value for %q: %w", key, err)
		}
		if _, ok := structTag.Lookup(key); !ok {
			return fmt.Errorf("reflect rejected struct tag key %q in %q", key, tag)
		}
		remaining = quoted[end+1:]
		if remaining != "" && remaining[0] != ' ' {
			return fmt.Errorf("struct tag values are not space-separated: %q", tag)
		}
	}
	return nil
}

type modelOutputTarget struct {
	stage     string
	target    string
	backup    string
	hadTarget bool
	prepared  bool
	installed bool
}

type modelOutputTransaction struct {
	backupRoot   string
	targets      []modelOutputTarget
	removeBackup func(string) error
	done         bool
}

// replaceModelOutputDirectories starts a transaction that swaps both staged
// trees into place. The caller must commit after all remaining work succeeds,
// or roll back to restore the previous DAO and Entity trees.
func replaceModelOutputDirectories(project Project, stageDAO, stageEntity string) (*modelOutputTransaction, error) {
	backupRoot, err := os.MkdirTemp(project.Root, ".gx-model-backup-")
	if err != nil {
		return nil, fmt.Errorf("create model backup directory: %w", err)
	}
	transaction := &modelOutputTransaction{backupRoot: backupRoot, removeBackup: os.RemoveAll, targets: []modelOutputTarget{
		{stage: stageDAO, target: project.Resolve(defaultModelOutput), backup: filepath.Join(backupRoot, "dao")},
		{stage: stageEntity, target: project.Resolve(defaultEntityOutput), backup: filepath.Join(backupRoot, "entity")},
	}}
	for index := range transaction.targets {
		target := &transaction.targets[index]
		if _, err := os.Stat(target.target); err == nil {
			if err := os.Rename(target.target, target.backup); err != nil {
				rollbackErr := transaction.Rollback()
				return nil, errors.Join(fmt.Errorf("stage existing model output %s: %w", target.target, err), rollbackErr)
			}
			target.hadTarget = true
		} else if !os.IsNotExist(err) {
			rollbackErr := transaction.Rollback()
			return nil, errors.Join(fmt.Errorf("stat model output %s: %w", target.target, err), rollbackErr)
		}
		target.prepared = true
	}
	for index := range transaction.targets {
		target := &transaction.targets[index]
		if err := os.MkdirAll(filepath.Dir(target.target), 0o755); err != nil {
			rollbackErr := transaction.Rollback()
			return nil, errors.Join(fmt.Errorf("create model output parent %s: %w", target.target, err), rollbackErr)
		}
		if err := os.Rename(target.stage, target.target); err != nil {
			rollbackErr := transaction.Rollback()
			return nil, errors.Join(fmt.Errorf("install generated model output %s: %w", target.target, err), rollbackErr)
		}
		target.installed = true
	}
	return transaction, nil
}

func (transaction *modelOutputTransaction) Rollback() error {
	if transaction == nil || transaction.done {
		return nil
	}
	transaction.done = true
	var rollbackErrors []error
	for index := len(transaction.targets) - 1; index >= 0; index-- {
		target := transaction.targets[index]
		if !target.prepared {
			continue
		}
		if target.installed {
			if err := os.RemoveAll(target.target); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("remove generated model output %s: %w", target.target, err))
				continue
			}
		}
		if target.hadTarget {
			if err := os.Rename(target.backup, target.target); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("restore model output %s: %w", target.target, err))
			}
		}
	}
	if len(rollbackErrors) == 0 {
		if err := os.RemoveAll(transaction.backupRoot); err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("remove model backup directory: %w", err))
		}
	}
	return errors.Join(rollbackErrors...)
}

func (transaction *modelOutputTransaction) Commit() error {
	if transaction == nil || transaction.done {
		return nil
	}
	transaction.done = true
	removeBackup := transaction.removeBackup
	if removeBackup == nil {
		removeBackup = os.RemoveAll
	}
	if err := removeBackup(transaction.backupRoot); err != nil {
		return fmt.Errorf("model output committed but remove backup %s: %w; backup retained", transaction.backupRoot, err)
	}
	return nil
}

// removeGeneratedImportAliases keeps generated code idiomatic and consistent
// with the other gx generators. GORM Gen may emit an explicit package alias
// when a schema directory name differs from the package name (for example,
// `entity ".../entity/public"`); the package declaration already supplies
// the correct identifier, so the alias is unnecessary.
func removeGeneratedImportAliases(roots ...string) error {
	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
			if os.IsNotExist(walkErr) {
				return filepath.SkipDir
			}
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			fileSet := token.NewFileSet()
			file, err := parser.ParseFile(fileSet, path, content, parser.ParseComments)
			if err != nil {
				return fmt.Errorf("parse generated model %s: %w", path, err)
			}
			changed := false
			for _, imported := range file.Imports {
				if imported.Name != nil && imported.Name.Name != "_" && imported.Name.Name != "." {
					imported.Name = nil
					changed = true
				}
			}
			if !changed {
				return nil
			}
			var formatted bytes.Buffer
			if err := format.Node(&formatted, fileSet, file); err != nil {
				return fmt.Errorf("format generated model %s: %w", path, err)
			}
			formatted.WriteByte('\n')
			return os.WriteFile(path, formatted.Bytes(), info.Mode().Perm())
		})
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove generated model import aliases in %s: %w", root, err)
		}
	}
	return nil
}

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

func moveGeneratedFiles(sourceRoot, destinationRoot string) error {
	return filepath.Walk(sourceRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(destinationRoot, relative)
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		if err := os.Rename(path, destination); err != nil {
			return err
		}
		return nil
	})
}

func projectImportPath(project Project, absolutePath string) string {
	relative, err := filepath.Rel(project.Root, absolutePath)
	if err != nil {
		return ""
	}
	return strings.TrimRight(project.ModulePath, "/") + "/" + filepath.ToSlash(relative)
}

func rewriteGeneratedImport(root, oldImport, newImport string) error {
	if oldImport == "" || newImport == "" || oldImport == newImport {
		return nil
	}
	return filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("parse generated import file %s: %w", path, err)
		}
		changed := false
		for _, imported := range file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return fmt.Errorf("unquote generated import in %s: %w", path, err)
			}
			if importPath != oldImport && !strings.HasPrefix(importPath, oldImport+"/") {
				continue
			}
			imported.Path.Value = strconv.Quote(newImport + strings.TrimPrefix(importPath, oldImport))
			changed = true
		}
		if !changed {
			return nil
		}
		var formatted bytes.Buffer
		if err := format.Node(&formatted, fileSet, file); err != nil {
			return fmt.Errorf("format generated import file %s: %w", path, err)
		}
		formatted.WriteByte('\n')
		return os.WriteFile(path, formatted.Bytes(), info.Mode().Perm())
	})
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

func ensureModelDependencies(project Project, result *Result) error {
	path := project.Resolve("go.mod")
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read project go.mod: %w", err)
	}
	file, err := modfile.Parse(path, content, nil)
	if err != nil {
		return fmt.Errorf("parse project go.mod: %w", err)
	}
	for _, dependency := range []struct {
		path    string
		version string
	}{
		{path: "gorm.io/gen", version: "v0.3.28"},
		{path: "gorm.io/plugin/dbresolver", version: "v1.5.3"},
		{path: "github.com/google/uuid", version: "v1.6.0"},
		{path: "github.com/shopspring/decimal", version: "v1.4.0"},
		{path: "gorm.io/datatypes", version: "v1.2.4"},
	} {
		file.AddRequire(dependency.path, dependency.version)
	}
	updated, err := file.Format()
	if err != nil {
		return fmt.Errorf("format project go.mod: %w", err)
	}
	if bytesEqual(content, updated) {
		return nil
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		return fmt.Errorf("update project go.mod: %w", err)
	}
	result.add("UPDATE", "go.mod", "added GORM model generation dependencies")
	return nil
}

func runGoModTidy(projectRoot string) error {
	command := exec.Command("go", "mod", "tidy")
	command.Dir = projectRoot
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("run go mod tidy: %w", err)
	}
	return nil
}

func snapshotModuleFiles(projectRoot string) (map[string][]byte, error) {
	return snapshotFiles(
		projectRoot,
		filepath.Join(projectRoot, "go.mod"),
		filepath.Join(projectRoot, "go.sum"),
	)
}

func restoreModuleFiles(projectRoot string, snapshot map[string][]byte) error {
	var restoreErrors []error
	for _, relative := range []string{"go.mod", "go.sum"} {
		path := filepath.Join(projectRoot, relative)
		content, existed := snapshot[relative]
		if !existed {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				restoreErrors = append(restoreErrors, fmt.Errorf("remove generated %s: %w", relative, err))
			}
			continue
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			restoreErrors = append(restoreErrors, fmt.Errorf("restore %s: %w", relative, err))
		}
	}
	return errors.Join(restoreErrors...)
}

func addModuleFileChanges(result *Result, before, after map[string][]byte) {
	paths := map[string]struct{}{
		"go.mod": {},
		"go.sum": {},
	}
	for path := range paths {
		beforeContent, existedBefore := before[path]
		afterContent, existsAfter := after[path]
		if existedBefore && existsAfter && bytesEqual(beforeContent, afterContent) {
			continue
		}
		kind := "UPDATE"
		switch {
		case !existedBefore && existsAfter:
			kind = "CREATE"
		case existedBefore && !existsAfter:
			kind = "DELETE"
		case !existedBefore && !existsAfter:
			continue
		}
		if hasChangePath(*result, path) {
			continue
		}
		result.add(kind, path, "go mod tidy")
	}
}

func hasChangePath(result Result, path string) bool {
	for _, change := range result.Changes {
		if change.Path == path {
			return true
		}
	}
	return false
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

func snapshotFiles(projectRoot string, roots ...string) (map[string][]byte, error) {
	result := make(map[string][]byte)
	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
			if os.IsNotExist(walkErr) {
				return filepath.SkipDir
			}
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(projectRoot, path)
			if err != nil {
				return err
			}
			result[filepath.Clean(relative)] = content
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("scan generated model files: %w", err)
		}
	}
	return result, nil
}

func addGeneratedModelHeaders(roots ...string) error {
	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
			if os.IsNotExist(walkErr) {
				return filepath.SkipDir
			}
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() || filepath.Ext(path) != ".go" {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if !generatedFile(content) && !bytes.Contains(content, []byte("Code generated by gorm.io/gen")) {
				return nil
			}
			updated := withGeneratedHeader(content)
			if bytes.Equal(content, updated) {
				return nil
			}
			return os.WriteFile(path, updated, info.Mode().Perm())
		})
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("add generated header to %s: %w", root, err)
		}
	}
	return nil
}

func addFileChanges(result *Result, before, after map[string][]byte) {
	paths := make(map[string]struct{}, len(before)+len(after))
	for path := range before {
		paths[path] = struct{}{}
	}
	for path := range after {
		paths[path] = struct{}{}
	}
	sortedPaths := make([]string, 0, len(paths))
	for path := range paths {
		sortedPaths = append(sortedPaths, path)
	}
	sort.Strings(sortedPaths)
	for _, path := range sortedPaths {
		_, existedBefore := before[path]
		_, existsAfter := after[path]
		switch {
		case !existedBefore && existsAfter:
			result.add("CREATE", path, "gorm model generated")
		case existedBefore && !existsAfter:
			result.add("DELETE", path, "gorm model removed")
		case existedBefore && existsAfter:
			result.add("UPDATE", path, "gorm model regenerated")
		}
	}
}

func bytesEqual(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// Package config provides the framework configuration abstraction and its
// Viper-backed implementation.
package config

import (
	"encoding"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
	"github.com/subosito/gotenv"
)

// Config is the framework configuration abstraction.
type Config interface {
	Get(key string) any
	GetString(key string) string
	GetInt(key string) int
	GetBool(key string) bool
	Unmarshal(target any) error
}

// ViperConfig adapts Viper without exposing it to framework users.
type ViperConfig struct {
	mu        sync.RWMutex
	viper     *viper.Viper
	root      string
	dotenv    map[string]string
	overrides map[string]any
	loadErr   error
	file      string
}

var defaultState struct {
	sync.Mutex
	root          string
	configuration *ViperConfig
	err           error
}

// New creates a configuration object with environment overrides enabled.
// Nested keys use underscores, for example server.address becomes
// SERVER_ADDRESS.
func New() *ViperConfig {
	root, err := projectRoot()
	if err != nil {
		root = "."
	}
	return newForRoot(root)
}

func newForRoot(root string) *ViperConfig {
	configuration := viper.New()
	configuration.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	configuration.AutomaticEnv()
	result := &ViperConfig{
		viper:     configuration,
		root:      root,
		dotenv:    make(map[string]string),
		overrides: make(map[string]any),
	}
	result.loadDotenv()
	return result
}

// Load reads YAML, JSON, TOML, or another Viper-supported format.
func Load(path string) (*ViperConfig, error) {
	configuration := New()
	if configuration.loadErr != nil {
		return nil, configuration.loadErr
	}
	configuration.viper.SetConfigFile(path)
	if err := configuration.viper.ReadInConfig(); err != nil {
		return nil, err
	}
	configuration.file = path
	return configuration, nil
}

// LoadDefault loads the conventional project configuration. It searches the
// project root first, then the config directory:
//
//	./config.yaml
//	./config/config.yaml
//	./manifest/config/config.yaml
//
// Missing files are valid and return an empty configuration. A malformed
// existing file is returned as an error.
func LoadDefault() (*ViperConfig, error) {
	root, err := projectRoot()
	if err != nil {
		return nil, err
	}
	return loadDefaultFromRoot(root)
}

func loadDefaultFromRoot(root string) (*ViperConfig, error) {
	configuration := newForRoot(root)
	if configuration.loadErr != nil {
		return configuration, configuration.loadErr
	}
	path := findDefaultConfig(root)
	if path == "" {
		return configuration, nil
	}
	configuration.viper.SetConfigFile(path)
	if err := configuration.viper.ReadInConfig(); err != nil {
		return configuration, fmt.Errorf("load config file %q: %w", path, err)
	}
	configuration.file = path
	return configuration, nil
}

// Default returns the process-wide project configuration. The configuration
// is initialized lazily and is refreshed when the process changes to another
// project root, which also keeps tests and embedded applications isolated.
func Default() *ViperConfig {
	configuration, _ := defaultConfiguration()
	return configuration
}

// Init initializes the process-wide project configuration and returns errors
// from an existing config file or .env file.
func Init() error {
	_, err := defaultConfiguration()
	return err
}

func defaultConfiguration() (*ViperConfig, error) {
	root, err := projectRoot()
	if err != nil {
		return newForRoot("."), err
	}
	defaultState.Lock()
	defer defaultState.Unlock()
	if defaultState.configuration != nil && defaultState.root == root {
		return defaultState.configuration, defaultState.err
	}
	configuration, loadErr := loadDefaultFromRoot(root)
	defaultState.root = root
	defaultState.configuration = configuration
	defaultState.err = loadErr
	return configuration, loadErr
}

// Get reads a value from the process-wide project configuration.
func Get(key string) any { return Default().Get(key) }

// GetString reads a string from the process-wide project configuration.
func GetString(key string) string { return Default().GetString(key) }

// GetInt reads an integer from the process-wide project configuration.
func GetInt(key string) int { return Default().GetInt(key) }

// GetBool reads a boolean from the process-wide project configuration.
func GetBool(key string) bool { return Default().GetBool(key) }

// Unmarshal decodes the process-wide project configuration into target.
func Unmarshal(target any) error { return Default().Unmarshal(target) }

// SetDefault adds a fallback value to the process-wide project configuration.
func SetDefault(key string, value any) { Default().SetDefault(key, value) }

// Set adds a runtime value to the process-wide project configuration.
func Set(key string, value any) { Default().Set(key, value) }

func (configuration *ViperConfig) Get(key string) any {
	configuration.mu.RLock()
	defer configuration.mu.RUnlock()
	if value, ok := configuration.externalValueLocked(key); ok {
		return value
	}
	return configuration.viper.Get(key)
}

func (configuration *ViperConfig) GetString(key string) string {
	configuration.mu.RLock()
	defer configuration.mu.RUnlock()
	if value, ok := configuration.externalValueLocked(key); ok {
		if value == nil {
			return ""
		}
		if text, ok := value.(string); ok {
			return text
		}
		return fmt.Sprint(value)
	}
	return configuration.viper.GetString(key)
}

func (configuration *ViperConfig) GetInt(key string) int {
	configuration.mu.RLock()
	defer configuration.mu.RUnlock()
	if value, ok := configuration.externalValueLocked(key); ok {
		return parseInt(value)
	}
	return configuration.viper.GetInt(key)
}

func (configuration *ViperConfig) GetBool(key string) bool {
	configuration.mu.RLock()
	defer configuration.mu.RUnlock()
	if value, ok := configuration.externalValueLocked(key); ok {
		return parseBool(value)
	}
	return configuration.viper.GetBool(key)
}

func (configuration *ViperConfig) Unmarshal(target any) error {
	// Viper's AutomaticEnv is applied by Get, but environment values are not
	// included in AllSettings/Unmarshal. Rebuild the known settings through
	// the effective getter so YAML, .env, system ENV, and Set are consistent.
	effective := viper.New()
	keys := configKeys(target)
	configuration.mu.RLock()
	seen := make(map[string]struct{})
	for _, key := range configuration.viper.AllKeys() {
		effective.Set(key, configuration.valueLocked(key))
		seen[strings.ToLower(key)] = struct{}{}
	}
	for _, key := range keys {
		normalizedKey := strings.ToLower(key)
		if _, ok := seen[normalizedKey]; !ok {
			if _, ok := configuration.externalValueLocked(key); ok {
				effective.Set(key, configuration.valueLocked(key))
			}
		}
	}
	configuration.mu.RUnlock()

	// Decoding can invoke target-defined TextUnmarshalers, so it must happen
	// after the configuration snapshot is complete and unlocked.
	return effective.Unmarshal(target, viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToWeakSliceHookFunc(","),
		mapstructure.TextUnmarshallerHookFunc(),
	)))
}

// SetDefault adds a fallback value.
func (configuration *ViperConfig) SetDefault(key string, value any) {
	configuration.mu.Lock()
	defer configuration.mu.Unlock()
	configuration.viper.SetDefault(key, value)
}

// Set adds a runtime value.
func (configuration *ViperConfig) Set(key string, value any) {
	configuration.mu.Lock()
	defer configuration.mu.Unlock()
	if configuration.overrides == nil {
		configuration.overrides = make(map[string]any)
	}
	configuration.overrides[strings.ToLower(strings.TrimSpace(key))] = value
	configuration.viper.Set(key, value)
}

// ConfigFile returns the file used by Load or LoadDefault. It is empty when
// no conventional config file exists.
func (configuration *ViperConfig) ConfigFile() string {
	if configuration == nil {
		return ""
	}
	configuration.mu.RLock()
	defer configuration.mu.RUnlock()
	return configuration.file
}

func (configuration *ViperConfig) loadDotenv() {
	path := filepath.Join(configuration.root, ".env")
	environment, err := gotenv.Read(path)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		configuration.loadErr = fmt.Errorf("load .env file %q: %w", path, err)
		return
	}
	for key, value := range environment {
		configuration.dotenv[strings.ToUpper(strings.TrimSpace(key))] = value
	}
}

func (configuration *ViperConfig) valueLocked(key string) any {
	if value, ok := configuration.externalValueLocked(key); ok {
		return value
	}
	return configuration.viper.Get(key)
}

func (configuration *ViperConfig) externalValueLocked(key string) (any, bool) {
	if configuration == nil {
		return nil, false
	}
	normalizedKey := strings.ToLower(strings.TrimSpace(key))
	if value, ok := configuration.overrides[normalizedKey]; ok {
		return value, true
	}
	keys := envKeys(key)
	for _, environmentKey := range keys {
		if value, ok := os.LookupEnv(environmentKey); ok {
			return value, true
		}
	}
	for _, environmentKey := range keys {
		if value, ok := configuration.dotenv[environmentKey]; ok {
			return value, true
		}
	}
	compactKey := compactEnvKey(keys[0])
	for _, entry := range os.Environ() {
		environmentKey, value, ok := strings.Cut(entry, "=")
		if ok && compactEnvKey(environmentKey) == compactKey {
			return value, true
		}
	}
	for environmentKey, value := range configuration.dotenv {
		if compactEnvKey(environmentKey) == compactKey {
			return value, true
		}
	}
	return nil, false
}

func configKeys(target any) []string {
	targetType := reflect.TypeOf(target)
	for targetType != nil && targetType.Kind() == reflect.Pointer {
		targetType = targetType.Elem()
	}
	if targetType == nil || targetType.Kind() != reflect.Struct {
		return nil
	}
	keys := make([]string, 0)
	collectConfigKeys(targetType, "", &keys, make(map[reflect.Type]struct{}))
	return keys
}

func collectConfigKeys(valueType reflect.Type, prefix string, keys *[]string, visiting map[reflect.Type]struct{}) {
	if isConfigLeafType(valueType) {
		if prefix != "" {
			*keys = append(*keys, prefix)
		}
		return
	}
	if _, ok := visiting[valueType]; ok {
		return
	}
	visiting[valueType] = struct{}{}
	defer delete(visiting, valueType)

	for index := 0; index < valueType.NumField(); index++ {
		field := valueType.Field(index)
		if !field.IsExported() {
			continue
		}
		tag := strings.Split(field.Tag.Get("mapstructure"), ",")
		name := tag[0]
		if name == "-" {
			continue
		}
		squash := false
		for _, option := range tag[1:] {
			if option == "squash" {
				squash = true
				break
			}
		}
		if name == "" {
			name = field.Name
		}
		key := name
		if prefix != "" {
			key = prefix + "." + name
		}
		fieldType := field.Type
		for fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}
		if fieldType.Kind() != reflect.Struct || isConfigLeafType(fieldType) {
			*keys = append(*keys, key)
			continue
		}
		if squash {
			key = prefix
		}
		collectConfigKeys(fieldType, key, keys, visiting)
	}
}

func isConfigLeafType(valueType reflect.Type) bool {
	if valueType.Kind() != reflect.Struct {
		return false
	}
	textUnmarshaler := reflect.TypeFor[encoding.TextUnmarshaler]()
	return valueType.Implements(textUnmarshaler) || reflect.PointerTo(valueType).Implements(textUnmarshaler)
}

func compactEnvKey(key string) string {
	return strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(key)), "_", "")
}

func parseInt(value any) int {
	switch value := value.(type) {
	case int:
		return value
	case int8:
		return int(value)
	case int16:
		return int(value)
	case int32:
		return int(value)
	case int64:
		return int(value)
	case uint:
		return int(value)
	case uint8:
		return int(value)
	case uint16:
		return int(value)
	case uint32:
		return int(value)
	case uint64:
		return int(value)
	case float32:
		return int(value)
	case float64:
		return int(value)
	}
	parsed, _ := strconv.Atoi(strings.TrimSpace(fmt.Sprint(value)))
	return parsed
}

func parseBool(value any) bool {
	if parsed, ok := value.(bool); ok {
		return parsed
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(fmt.Sprint(value)))
	if err == nil {
		return parsed
	}
	switch strings.ToLower(strings.TrimSpace(fmt.Sprint(value))) {
	case "1", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func envKeys(key string) []string {
	key = strings.TrimSpace(key)
	return []string{strings.ToUpper(camelToSnake(strings.NewReplacer(".", "_", "-", "_").Replace(key)))}
}

func camelToSnake(value string) string {
	runes := []rune(value)
	var result strings.Builder
	for index, character := range runes {
		if unicode.IsUpper(character) && index > 0 {
			previous := runes[index-1]
			var next rune
			if index+1 < len(runes) {
				next = runes[index+1]
			}
			if unicode.IsLower(previous) || unicode.IsDigit(previous) || (unicode.IsUpper(previous) && unicode.IsLower(next)) {
				result.WriteByte('_')
			}
		}
		result.WriteRune(unicode.ToLower(character))
	}
	return result.String()
}

func projectRoot() (string, error) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return findProjectRoot(workingDirectory), nil
}

func findProjectRoot(start string) string {
	start, err := filepath.Abs(start)
	if err != nil {
		return start
	}
	if info, statErr := os.Stat(start); statErr == nil && !info.IsDir() {
		start = filepath.Dir(start)
	}
	for directory := start; ; directory = filepath.Dir(directory) {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return start
		}
	}
}

func findDefaultConfig(root string) string {
	for _, relative := range []string{
		"config.yaml",
		filepath.Join("config", "config.yaml"),
		filepath.Join("manifest", "config", "config.yaml"),
	} {
		path := filepath.Join(root, relative)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

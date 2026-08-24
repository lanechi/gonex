package config

import (
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

type textConfigValue struct {
	value string
}

func (value *textConfigValue) UnmarshalText(text []byte) error {
	value.value = string(text)
	return nil
}

type reentrantTextConfigValue struct {
	value string
}

var reentrantConfiguration *ViperConfig

func (value *reentrantTextConfigValue) UnmarshalText(text []byte) error {
	reentrantConfiguration.Set("decoded", true)
	value.value = string(text)
	return nil
}

func TestLoadDefaultPriorityAndFallbacks(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/config-test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte("server:\n  address: ':root'\n  maxBodyBytes: 100\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "config.yaml"), []byte("server:\n  address: ':config'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("SERVER_ADDRESS=:dotenv\nSERVER_MAX_BODY_BYTES=200\nFEATURE_ENABLED=true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	t.Setenv("SERVER_ADDRESS", ":system")

	configuration, err := LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.ConfigFile() != filepath.Join(root, "config.yaml") {
		t.Fatalf("config file=%q", configuration.ConfigFile())
	}
	if got := configuration.GetString("server.address"); got != ":system" {
		t.Fatalf("system ENV did not win: %q", got)
	}
	if got := configuration.GetInt("server.maxBodyBytes"); got != 200 {
		t.Fatalf(".env did not override YAML: %d", got)
	}
	if !configuration.GetBool("feature.enabled") {
		t.Fatal(".env boolean was not loaded")
	}

	configuration.SetDefault("server.port", 9000)
	if got := configuration.GetInt("server.port"); got != 9000 {
		t.Fatalf("default value=%d", got)
	}
	configuration.Set("server.address", ":runtime")
	if got := configuration.GetString("server.address"); got != ":runtime" {
		t.Fatalf("runtime value=%q", got)
	}

	var values struct {
		Server struct {
			Address      string `mapstructure:"address"`
			MaxBodyBytes int    `mapstructure:"maxBodyBytes"`
		} `mapstructure:"server"`
	}
	if err := configuration.Unmarshal(&values); err != nil {
		t.Fatal(err)
	}
	if values.Server.Address != ":runtime" || values.Server.MaxBodyBytes != 200 {
		t.Fatalf("unmarshaled values=%+v", values)
	}
}

func TestUnmarshalReadsEnvironmentOnlyNestedValues(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/environment-config-test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	t.Setenv("FEATURE_ENABLED", "true")

	configuration, err := LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	var values struct {
		Feature struct {
			Enabled bool `mapstructure:"enabled"`
		} `mapstructure:"feature"`
	}
	if err := configuration.Unmarshal(&values); err != nil {
		t.Fatal(err)
	}
	if !values.Feature.Enabled {
		t.Fatal("environment-only nested value was not unmarshaled")
	}
}

func TestUnmarshalReadsDotenvOnlyNestedValues(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/dotenv-config-test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("FEATURE_ENABLED=true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	configuration, err := LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	var values struct {
		Feature struct {
			Enabled bool `mapstructure:"enabled"`
		} `mapstructure:"feature"`
	}
	if err := configuration.Unmarshal(&values); err != nil {
		t.Fatal(err)
	}
	if !values.Feature.Enabled {
		t.Fatal("dotenv-only nested value was not unmarshaled")
	}
}

func TestUnmarshalDecodesEnvironmentOnlyTextUnmarshalers(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/environment-text-config-test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	t.Setenv("CONFIG_WHEN", "2026-08-23T12:34:56Z")
	t.Setenv("CONFIG_TEXT", "environment")

	configuration, err := LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	var values struct {
		Config struct {
			When time.Time       `mapstructure:"when"`
			Text textConfigValue `mapstructure:"text"`
		} `mapstructure:"config"`
	}
	if err := configuration.Unmarshal(&values); err != nil {
		t.Fatal(err)
	}
	if got, want := values.Config.When.Format(time.RFC3339), "2026-08-23T12:34:56Z"; got != want {
		t.Fatalf("when=%q, want %q", got, want)
	}
	if got, want := values.Config.Text.value, "environment"; got != want {
		t.Fatalf("text=%q, want %q", got, want)
	}
}

func TestUnmarshalDecodesDotenvOnlyTextUnmarshalers(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/dotenv-text-config-test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("CONFIG_WHEN=2026-08-23T12:34:56Z\nCONFIG_TEXT=dotenv\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	configuration, err := LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	var values struct {
		Config struct {
			When time.Time       `mapstructure:"when"`
			Text textConfigValue `mapstructure:"text"`
		} `mapstructure:"config"`
	}
	if err := configuration.Unmarshal(&values); err != nil {
		t.Fatal(err)
	}
	if got, want := values.Config.When.Format(time.RFC3339), "2026-08-23T12:34:56Z"; got != want {
		t.Fatalf("when=%q, want %q", got, want)
	}
	if got, want := values.Config.Text.value, "dotenv"; got != want {
		t.Fatalf("text=%q, want %q", got, want)
	}
}

func TestUnmarshalTextUnmarshalerCanReenterConfiguration(t *testing.T) {
	configuration := New()
	configuration.Set("value", "reentrant")
	reentrantConfiguration = configuration
	t.Cleanup(func() { reentrantConfiguration = nil })

	result := make(chan error, 1)
	go func() {
		var values struct {
			Value reentrantTextConfigValue `mapstructure:"value"`
		}
		result <- configuration.Unmarshal(&values)
	}()

	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Unmarshal held the configuration lock while decoding")
	}
	if !configuration.GetBool("decoded") {
		t.Fatal("reentrant TextUnmarshaler did not update configuration")
	}
}

func TestConcurrentSetGetAndUnmarshal(t *testing.T) {
	configuration := New()
	const workers = 16
	const iterations = 100
	var group sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		group.Add(1)
		go func(worker int) {
			defer group.Done()
			for iteration := 0; iteration < iterations; iteration++ {
				configuration.Set("feature.enabled", (worker+iteration)%2 == 0)
				_ = configuration.GetBool("feature.enabled")
				var values struct {
					Feature struct {
						Enabled bool `mapstructure:"enabled"`
					} `mapstructure:"feature"`
				}
				if err := configuration.Unmarshal(&values); err != nil {
					t.Errorf("Unmarshal: %v", err)
					return
				}
			}
		}(worker)
	}
	group.Wait()
}

func TestConfigKeysHandlesMapstructureOptionsAndRecursiveTypes(t *testing.T) {
	type Squashed struct {
		Value string `mapstructure:"value"`
	}
	type Named struct {
		Value string `mapstructure:"value"`
	}
	type Plain struct {
		Value string `mapstructure:"value"`
	}
	type Recursive struct {
		Enabled bool       `mapstructure:"enabled"`
		Next    *Recursive `mapstructure:"next"`
	}
	type Values struct {
		Squashed `mapstructure:",squash"`
		Plain
		Named   `mapstructure:"named"`
		Ignored string          `mapstructure:"-,omitempty"`
		When    time.Time       `mapstructure:"when"`
		Text    textConfigValue `mapstructure:"text"`
		Tree    Recursive       `mapstructure:"tree"`
	}

	got := configKeys(&Values{})
	want := []string{"value", "Plain.value", "named.value", "when", "text", "tree.enabled"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("config keys=%v, want %v", got, want)
	}
}

func TestLoadDefaultWithoutFilesIsAllowed(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/empty-config-test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	configuration, err := LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.ConfigFile() != "" {
		t.Fatalf("unexpected config file=%q", configuration.ConfigFile())
	}
	if got := configuration.GetString("missing"); got != "" {
		t.Fatalf("missing string=%q", got)
	}
}

func TestDefaultGlobalConfigurationFollowsProjectRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/global-config-test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("APP_NAME=global\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	if err := Init(); err != nil {
		t.Fatal(err)
	}
	if got := GetString("app.name"); got != "global" {
		t.Fatalf("global config value=%q", got)
	}
}

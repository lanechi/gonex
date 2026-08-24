package test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanechi/gonex/gx/internal/cli"
)

func TestCLICommands(t *testing.T) {
	for _, args := range [][]string{
		{"--help"},
		{"completion", "bash"},
		{"init", "--help"},
		{"ctrl", "--help"},
		{"service", "--help"},
		{"dao", "--help"},
	} {
		t.Run(strings.Join(args, "-"), func(t *testing.T) {
			if err := cli.Run(args); err != nil {
				t.Fatalf("gx %s: %v", strings.Join(args, " "), err)
			}
		})
	}
}

func TestCLIRejectsRemovedAndUnsupportedOptions(t *testing.T) {
	for _, test := range []struct {
		args string
		want string
	}{
		{args: "gen", want: `unknown command "gen"`},
		{args: "ctrl --src other-api", want: "unknown flag: --src"},
		{args: "service --dst other-service", want: "unknown flag: --dst"},
		{args: "dao --config database.yaml", want: "unknown flag: --config"},
	} {
		t.Run(strings.ReplaceAll(test.args, " ", "-"), func(t *testing.T) {
			if err := cli.Run(strings.Fields(test.args)); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("gx %s error = %v, want %q", test.args, err, test.want)
			}
		})
	}
}

func TestCLIUsesCanonicalDemoDryRun(t *testing.T) {
	t.Chdir(filepath.Join(repositoryRoot(), "examples", "demo"))
	if err := cli.Run([]string{"ctrl", "--dry-run"}); err != nil {
		t.Fatalf("gx ctrl --dry-run: %v", err)
	}
	if err := cli.Run([]string{"service", "--dry-run"}); err != nil {
		t.Fatalf("gx service --dry-run: %v", err)
	}
}

func TestCLIInitDryRunDoesNotNeedNetwork(t *testing.T) {
	target := filepath.Join(t.TempDir(), "app")
	if err := cli.Run([]string{"init", target, "--module", "example.com/app", "--dry-run"}); err != nil {
		t.Fatalf("gx init --dry-run: %v", err)
	}
}

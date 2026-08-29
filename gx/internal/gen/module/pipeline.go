// Package module owns safe module-template extraction and publication phases.
package module

import (
	"fmt"
	"os"
)

// Discovery is the validated target and template selection for initialization.
type Discovery struct {
	Target  string
	Name    string
	URL     string
	Options InitOptions
}

// Extracted contains the canonical demo after safe extraction to staging.
type Extracted struct {
	Discovery Discovery
	Staging   string
}

// Rewritten contains a staging project with its module and project identifiers
// replaced.
type Rewritten struct{ Extracted Extracted }

// Validated contains a rewritten demo that passed the initialization manifest.
type Validated struct{ Rewritten Rewritten }

// Staged contains the complete initialization publication plan.
type Staged struct {
	Validated Validated
	Result    Result
}

// Pipeline runs initialization through explicit stage products.
type Pipeline struct{}

// Run performs Discovery→Extracted→Rewritten→Validated→Staged→Commit.
func (Pipeline) Run(target string, options InitOptions) (Result, error) {
	discovery, err := discover(target, options)
	if err != nil {
		return Result{}, fmt.Errorf("module discover: %w", err)
	}
	extracted, err := extract(discovery)
	if err != nil {
		return Result{}, fmt.Errorf("module extract: %w", err)
	}
	defer func() {
		if extracted.Staging != "" {
			_ = os.RemoveAll(extracted.Staging)
		}
	}()
	rewritten, err := rewrite(extracted)
	if err != nil {
		return Result{}, fmt.Errorf("module rewrite: %w", err)
	}
	validated, err := validate(rewritten)
	if err != nil {
		return Result{}, fmt.Errorf("module validate: %w", err)
	}
	staged := stage(validated)
	if err := commit(&staged); err != nil {
		return Result{}, fmt.Errorf("module commit: %w", err)
	}
	return staged.Result, nil
}

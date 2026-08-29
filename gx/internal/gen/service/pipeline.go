// Package service owns the service generation stage contract.
package service

import (
	"fmt"

	genfs "github.com/lanechi/gonex/gx/internal/gen/fs"
	"github.com/lanechi/gonex/gx/internal/gen/shared"
)

// Discovery is the validated Logic input to service generation.
type Discovery struct {
	Project Project
	Options ServiceOptions
}

// Rendered contains every service, Logic, model, and aggregator candidate
// before formatting and ownership checks.
type Rendered struct {
	Discovery Discovery
	Outputs   []shared.Output
}

// Formatted contains the complete ownership-approved publication set.
type Formatted struct {
	Rendered Rendered
	Result   Result
	Outputs  []shared.PreparedOutput
}

// Staged owns the single transaction covering one service invocation.
type Staged struct {
	Formatted   Formatted
	Transaction *genfs.Transaction
}

// Pipeline runs service generation through explicit data products.
type Pipeline struct{}

// Run performs Discovery→Rendered→Formatted→Staged→Commit and stops at the
// first failed transition.
func (Pipeline) Run(discovery Discovery) (Result, error) {
	rendered, err := render(discovery)
	if err != nil {
		return Result{}, fmt.Errorf("service render: %w", err)
	}
	return (Pipeline{}).Commit(rendered)
}

// Commit formats, stages, and commits an already rendered service product.
func (Pipeline) Commit(rendered Rendered) (Result, error) {
	formatted, err := formatRendered(rendered)
	if err != nil {
		return Result{}, fmt.Errorf("service format: %w", err)
	}
	staged, err := stage(formatted)
	if err != nil {
		return Result{}, fmt.Errorf("service stage: %w", err)
	}
	defer func() {
		if staged.Transaction != nil {
			_ = staged.Transaction.Rollback()
		}
	}()
	if err := commit(staged); err != nil {
		return Result{}, fmt.Errorf("service commit: %w", err)
	}
	return formatted.Result, nil
}

func formatRendered(rendered Rendered) (Formatted, error) {
	result, outputs, err := shared.PrepareOutputs(rendered.Discovery.Project, rendered.Outputs, rendered.Discovery.Options.DryRun)
	return Formatted{Rendered: rendered, Result: result, Outputs: outputs}, err
}

func stage(formatted Formatted) (Staged, error) {
	transaction, err := shared.StageOutputs(formatted.Rendered.Discovery.Project, formatted.Outputs, formatted.Rendered.Discovery.Options.DryRun)
	return Staged{Formatted: formatted, Transaction: transaction}, err
}

func commit(staged Staged) error {
	if staged.Transaction == nil {
		return nil
	}
	return staged.Transaction.Commit()
}

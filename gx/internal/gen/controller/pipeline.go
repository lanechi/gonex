// Package controller owns the controller generation stage contract.
package controller

import (
	"fmt"

	genfs "github.com/lanechi/gonex/gx/internal/gen/fs"
	"github.com/lanechi/gonex/gx/internal/gen/shared"
)

// Discovery is the validated API input to controller generation.
type Discovery struct {
	Project Project
	Options ControllerOptions
	APIs    []API
}

// Rendered contains every controller file before formatting and ownership checks.
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

// Staged owns the single transaction covering one controller invocation.
type Staged struct {
	Formatted   Formatted
	Transaction *genfs.Transaction
}

// Pipeline runs controller generation through explicit data products.
type Pipeline struct{}

// Run performs Discovery→Rendered→Formatted→Staged→Publish and stops at the first failed transition.
func (Pipeline) Run(discovery Discovery) (Result, error) {
	rendered, err := render(discovery)
	if err != nil {
		return Result{}, fmt.Errorf("controller render: %w", err)
	}
	return (Pipeline{}).Publish(rendered)
}

// Publish formats, stages, and commits an already rendered controller product.
func (Pipeline) Publish(rendered Rendered) (Result, error) {
	formatted, err := formatRendered(rendered)
	if err != nil {
		return Result{}, fmt.Errorf("controller format: %w", err)
	}
	staged, err := stage(formatted)
	if err != nil {
		return Result{}, fmt.Errorf("controller stage: %w", err)
	}
	defer func() {
		if staged.Transaction != nil {
			_ = staged.Transaction.Rollback()
		}
	}()
	if err := publish(staged); err != nil {
		return Result{}, fmt.Errorf("controller publish: %w", err)
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

func publish(staged Staged) error {
	if staged.Transaction == nil {
		return nil
	}
	return staged.Transaction.Commit()
}

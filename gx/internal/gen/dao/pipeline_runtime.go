package dao

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	genfs "github.com/lanechi/gonex/gx/internal/gen/fs"
)

// Generate introspects the database and transactionally replaces DAO and Entity output.
func Generate(project Project, options ModelOptions) (Result, error) {
	return (Pipeline{}).Run(project, options)
}

// Run performs Discovery→Generated→Rendered→Formatted→Validated→Staged→Commit.
func (Pipeline) Run(project Project, options ModelOptions) (result Result, err error) {
	discovery, err := discover(project, options)
	if err != nil {
		return Result{}, fmt.Errorf("dao discover: %w", err)
	}
	defer func() {
		if discovery.closeDatabase != nil {
			_ = discovery.closeDatabase()
		}
	}()

	generated, err := generate(discovery)
	if err != nil {
		return Result{}, fmt.Errorf("dao generate: %w", err)
	}
	defer func() { _ = os.RemoveAll(generated.StageRoot) }()
	rendered, err := render(generated)
	if err != nil {
		return Result{}, fmt.Errorf("dao render: %w", err)
	}
	formatted, err := formatRendered(rendered)
	if err != nil {
		return Result{}, fmt.Errorf("dao format: %w", err)
	}
	validated, err := validate(formatted)
	if err != nil {
		return Result{}, fmt.Errorf("dao validate: %w", err)
	}
	staged, err := stage(validated)
	if err != nil {
		return Result{}, fmt.Errorf("dao stage: %w", err)
	}
	defer func() {
		if err == nil {
			return
		}
		if rollbackErr := rollbackFailedStage(discovery.Project.Root, discovery.ModuleBefore, staged.Transaction); rollbackErr != nil {
			err = errors.Join(err, rollbackErr)
		}
	}()
	if err := commit(&staged); err != nil {
		return Result{}, fmt.Errorf("dao commit: %w", err)
	}
	return staged.Result, nil
}

// rollbackFailedStage restores directory and module state only while publication
// is still reversible. DirectoryTransaction.Commit marks publication committed
// before backup/root-handle cleanup, so a cleanup-only failure must not restore
// the old go.mod/go.sum while leaving the new DAO/Entity directories published.
func rollbackFailedStage(projectRoot string, moduleBefore map[string][]byte, transaction *genfs.DirectoryTransaction) error {
	if transaction == nil || transaction.Committed() {
		return nil
	}
	return errors.Join(transaction.Rollback(), restoreModuleFiles(projectRoot, moduleBefore))
}

func discover(project Project, options ModelOptions) (Discovery, error) {
	moduleBefore, err := snapshotModuleFiles(project.Root)
	if err != nil {
		return Discovery{}, err
	}
	config, err := LoadDatabaseEnv(project.Root)
	if err != nil {
		return Discovery{}, err
	}
	resolveDatabasePaths(project.Root, &config)
	database, err := openConfiguredDatabase(config)
	if err != nil {
		return Discovery{}, err
	}
	discovery := Discovery{Project: project, Options: options, Config: config, Database: database, ModuleBefore: moduleBefore, OutputRoot: project.Resolve(defaultModelOutput), ModelRoot: project.Resolve(defaultEntityOutput)}
	if sqlDatabase, dbErr := database.DB(); dbErr == nil {
		discovery.closeDatabase = sqlDatabase.Close
	}
	discovery.Before, err = snapshotFiles(project.Root, discovery.OutputRoot, discovery.ModelRoot)
	if err != nil {
		if discovery.closeDatabase != nil {
			_ = discovery.closeDatabase()
		}
		return Discovery{}, err
	}
	return discovery, nil
}

func generate(discovery Discovery) (Generated, error) {
	stageRoot, err := os.MkdirTemp(discovery.Project.Root, "gx-model-stage-")
	if err != nil {
		return Generated{}, fmt.Errorf("create model generation staging directory: %w", err)
	}
	generated := Generated{Discovery: discovery, StageRoot: stageRoot, StageDAO: filepath.Join(stageRoot, "dao"), StageEntity: filepath.Join(stageRoot, "entity")}
	if isPostgresDriver(discovery.Config.Driver) {
		err = generatePostgresModels(discovery.Project, discovery.Database, discovery.Options.Tables, generated.StageDAO, generated.StageEntity)
	} else {
		err = generateModels(discovery.Database, generated.StageDAO, generated.StageEntity, splitTables(discovery.Options.Tables))
	}
	if err != nil {
		_ = os.RemoveAll(stageRoot)
		return Generated{}, err
	}
	return generated, nil
}

func render(generated Generated) (Rendered, error) {
	if err := removeGeneratedImportAliases(generated.StageDAO, generated.StageEntity); err != nil {
		return Rendered{}, err
	}
	if err := rewriteGeneratedImport(generated.StageDAO, projectImportPath(generated.Discovery.Project, generated.StageEntity), projectImportPath(generated.Discovery.Project, generated.Discovery.ModelRoot)); err != nil {
		return Rendered{}, fmt.Errorf("rewrite staged DAO entity import: %w", err)
	}
	if err := addGeneratedModelHeaders(generated.StageDAO, generated.StageEntity); err != nil {
		return Rendered{}, err
	}
	if err := sanitizeGeneratedStructTags(generated.StageDAO, generated.StageEntity); err != nil {
		return Rendered{}, err
	}
	return Rendered{Generated: generated}, nil
}

func formatRendered(rendered Rendered) (Formatted, error) {
	if err := formatGeneratedModelOutput(rendered.Generated.StageDAO, rendered.Generated.StageEntity); err != nil {
		return Formatted{}, err
	}
	return Formatted{Rendered: rendered}, nil
}

func validate(formatted Formatted) (Validated, error) {
	if err := validateGeneratedModelOutput(formatted.Rendered.Generated.StageDAO, formatted.Rendered.Generated.StageEntity); err != nil {
		return Validated{}, err
	}
	return Validated{Formatted: formatted}, nil
}

func stage(validated Validated) (Staged, error) {
	generated := validated.Formatted.Rendered.Generated
	transaction, err := genfs.BeginDirectoryTransaction(generated.Discovery.Project.Root,
		genfs.DirectorySwap{Stage: generated.StageDAO, Target: defaultModelOutput},
		genfs.DirectorySwap{Stage: generated.StageEntity, Target: defaultEntityOutput},
	)
	if err != nil {
		return Staged{}, err
	}
	after, err := snapshotFiles(generated.Discovery.Project.Root, generated.Discovery.OutputRoot, generated.Discovery.ModelRoot)
	if err != nil {
		_ = transaction.Rollback()
		return Staged{}, err
	}
	staged := Staged{Validated: validated, Transaction: transaction}
	addFileChanges(&staged.Result, generated.Discovery.Before, after)
	return staged, nil
}

func commit(staged *Staged) error {
	if staged == nil {
		return fmt.Errorf("staged DAO product is required")
	}
	discovery := staged.Validated.Formatted.Rendered.Generated.Discovery
	if err := ensureModelDependencies(discovery.Project, &staged.Result); err != nil {
		return err
	}
	tidy := discovery.Options.runTidy
	if tidy == nil {
		tidy = runGoModTidy
	}
	if err := tidy(discovery.Project.Root); err != nil {
		return err
	}
	moduleAfter, err := snapshotModuleFiles(discovery.Project.Root)
	if err != nil {
		return err
	}
	addModuleFileChanges(&staged.Result, discovery.ModuleBefore, moduleAfter)
	if staged.Transaction == nil {
		return nil
	}
	return staged.Transaction.Commit()
}

// Package dao owns the transactional DAO generation stage contract.
package dao

import (
	genfs "github.com/lanechi/gonex/gx/internal/gen/fs"
	"gorm.io/gorm"
)

// Discovery is the validated database and project state for DAO generation.
type Discovery struct {
	Project       Project
	Options       ModelOptions
	Config        DatabaseConfig
	Database      *gorm.DB
	ModuleBefore  map[string][]byte
	OutputRoot    string
	ModelRoot     string
	Before        map[string][]byte
	closeDatabase func() error
}

// Generated contains gorm's unformatted DAO and Entity output in its staging
// directory.
type Generated struct {
	Discovery   Discovery
	StageRoot   string
	StageDAO    string
	StageEntity string
}

// Rendered contains generated output after gx-specific import and ownership
// rewrites.
type Rendered struct{ Generated Generated }

// Formatted contains gofmt-normalized generated output.
type Formatted struct{ Rendered Rendered }

// Validated contains generated output that passed source and struct-tag checks.
type Validated struct{ Formatted Formatted }

// Staged owns the paired DAO and Entity directory transaction until commit or
// rollback completes.
type Staged struct {
	Validated   Validated
	Transaction *genfs.DirectoryTransaction
	Result      Result
}

// Pipeline runs DAO generation through explicit stage products.
type Pipeline struct{}

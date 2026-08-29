// Package gen exposes gx's stable generator API. Domain implementations live
// in controller, service, dao, and module; shared values live in shared.
package gen

import (
	"github.com/lanechi/gonex/gx/internal/gen/controller"
	"github.com/lanechi/gonex/gx/internal/gen/dao"
	"github.com/lanechi/gonex/gx/internal/gen/module"
	"github.com/lanechi/gonex/gx/internal/gen/service"
	"github.com/lanechi/gonex/gx/internal/gen/shared"
)

type (
	Project           = shared.Project
	Change            = shared.Change
	Result            = shared.Result
	ControllerOptions = shared.ControllerOptions
	ServiceOptions    = shared.ServiceOptions
	API               = shared.API
	LogicMethod       = shared.LogicMethod
	ImportRef         = shared.ImportRef
	ModelOptions      = dao.ModelOptions
	DatabaseConfig    = dao.DatabaseConfig
	InitOptions       = module.InitOptions
)

// DiscoverProject finds the nearest module from start.
func DiscoverProject(start string) (Project, error) { return shared.DiscoverProject(start) }

// GenerateControllers delegates Controller generation to its domain pipeline.
func GenerateControllers(project Project, options ControllerOptions) (Result, error) {
	return controller.Generate(project, options)
}

// GenerateServices delegates Service generation to its domain pipeline.
func GenerateServices(project Project, options ServiceOptions) (Result, error) {
	return service.Generate(project, options)
}

// GenerateModels delegates database model generation to its domain pipeline.
func GenerateModels(project Project, options ModelOptions) (Result, error) {
	return dao.Generate(project, options)
}

// LoadDatabaseEnv reads database configuration for the DAO generator.
func LoadDatabaseEnv(projectRoot string) (DatabaseConfig, error) {
	return dao.LoadDatabaseEnv(projectRoot)
}

// InitProject delegates canonical template initialization to its domain pipeline.
func InitProject(target string, options InitOptions) (Result, error) {
	return module.Init(target, options)
}

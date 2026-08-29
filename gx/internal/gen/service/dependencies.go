package service

import (
	"github.com/lanechi/gonex/gx/internal/gen/controller"
	"github.com/lanechi/gonex/gx/internal/gen/shared"
)

type (
	Project        = shared.Project
	Result         = shared.Result
	ServiceOptions = shared.ServiceOptions
	LogicMethod    = shared.LogicMethod
	ImportRef      = shared.ImportRef
)

const (
	defaultLogicSource   = "internal/logic"
	defaultDemoModelPath = "internal/model/testmodel.go"
	defaultServiceDest   = "internal/service"
	logicAggregatorName  = "logic.go"
	generatedHeader      = shared.GeneratedHeader
)

func withGeneratedHeader(source []byte) []byte { return shared.WithGeneratedHeader(source) }

func validIdentifier(value string) bool      { return controller.ValidIdentifier(value) }
func exportedIdentifier(value string) string { return controller.ExportedIdentifier(value) }
func strconvQuote(value string) string       { return controller.StrconvQuote(value) }

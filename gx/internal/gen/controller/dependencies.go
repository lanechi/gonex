package controller

import "github.com/lanechi/gonex/gx/internal/gen/shared"

type (
	Project           = shared.Project
	Result            = shared.Result
	API               = shared.API
	ControllerOptions = shared.ControllerOptions
)

const generatedHeader = shared.GeneratedHeader

const (
	defaultAPISource      = "api"
	defaultControllerDest = "internal/controller"
)

func generatedFile(source []byte) bool { return shared.IsGeneratedFile(source) }

// ValidIdentifier reports whether value is a valid Go identifier.
func ValidIdentifier(value string) bool { return validIdentifier(value) }

// ExportedIdentifier converts a file-style identifier to its exported Go form.
func ExportedIdentifier(value string) string { return exportedIdentifier(value) }

// StrconvQuote renders a Go string literal.
func StrconvQuote(value string) string { return strconvQuote(value) }

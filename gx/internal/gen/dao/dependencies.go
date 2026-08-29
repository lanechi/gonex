package dao

import "github.com/lanechi/gonex/gx/internal/gen/shared"

type (
	Project = shared.Project
	Result  = shared.Result
)

func generatedFile(source []byte) bool         { return shared.IsGeneratedFile(source) }
func withGeneratedHeader(source []byte) []byte { return shared.WithGeneratedHeader(source) }

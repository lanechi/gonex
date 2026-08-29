package ghttp

import (
	"github.com/lanechi/gonex/ghttp/internal/ginruntime"
	"github.com/lanechi/gonex/logging"
)

var ginRouteRegistrationMu = &ginruntime.RegistrationMu

const Named = "[GIN-debug]"

// configureGinLogging routes Gin's two package-level writers through the
// framework logger. Gin is an internal implementation detail of ghttp, so no
// Gin adapter is exposed from logging.
func configureGinLogging(logger logging.Logger) {
	ginruntime.ConfigureLogging(logger)
}

func installGinLogging(logger logging.Logger) {
	ginruntime.InstallLogging(logger)
}

package ghttp

import "github.com/lanechi/gonex/router"

func scanController(controller any) ([]router.Definition, error) {
	return router.ScanController(controller)
}

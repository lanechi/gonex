package hello

import (
	"context"
	"strings"

	"github.com/lanechi/gonex/examples/demo/internal/service"
)

type logic struct{}

func init()               { service.RegisterHello(New()) }
func New() service.IHello { return &logic{} }
func (*logic) Greet(_ context.Context, name string) string {
	if strings.TrimSpace(name) == "" {
		return "Hello, gonex!"
	}
	return "Hello, " + name + "!"
}

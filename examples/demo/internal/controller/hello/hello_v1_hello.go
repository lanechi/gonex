package hello

import (
	"context"

	"github.com/lanechi/gonex/examples/demo/api/hello/v1"
	"github.com/lanechi/gonex/examples/demo/internal/service"
)

func (*ControllerV1) Hello(ctx context.Context, req *v1.HelloReq) (*v1.HelloRes, error) {
	return &v1.HelloRes{Message: service.Hello().Greet(ctx, req.Name)}, nil
}

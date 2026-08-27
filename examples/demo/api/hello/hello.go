package hello

import (
	"context"

	v1 "github.com/lanechi/gonex/examples/demo/api/hello/v1"
)

type IHelloV1 interface {
	Hello(ctx context.Context, req *v1.HelloReq) (*v1.HelloRes, error)
}

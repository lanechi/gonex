package hello_test

import (
	"context"
	"testing"

	v1 "github.com/lanechi/gonex/examples/demo/api/hello/v1"
	"github.com/lanechi/gonex/examples/demo/internal/controller/hello"
	_ "github.com/lanechi/gonex/examples/demo/internal/logic"
)

func TestHelloUsesRegisteredService(t *testing.T) {
	response, err := new(hello.ControllerV1).Hello(context.Background(), &v1.HelloReq{Name: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	if response.Message != "Hello, Ada!" {
		t.Fatalf("message = %q", response.Message)
	}
}

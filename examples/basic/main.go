package main

import (
	"context"

	"github.com/lanechi/gonex/g"
	"github.com/lanechi/gonex/ghttp"
)

type HelloReq struct {
	g.Meta `       path:"/hello" method:"get" tags:"Hello" summary:"Say hello"`
	Name   string `                                                            query:"name"`
}

type HelloRes struct {
	Message string `json:"message"`
}

type HelloController struct{}

func (*HelloController) Hello(_ context.Context, req *HelloReq) (*HelloRes, error) {
	name := req.Name
	if name == "" {
		name = "gonex"
	}
	return &HelloRes{Message: "Hello, " + name + "!"}, nil
}

func main() {
	server := ghttp.NewServer(ghttp.WithMode(ghttp.DebugMode))

	server.Group("/", func(group *ghttp.RouterGroup) {
		if err := group.Bind(&HelloController{}); err != nil {
			panic(err)
		}
	})
	if err := server.Run(); err != nil {
		panic(err)
	}
}

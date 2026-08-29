package main

import (
	"context"
	"time"

	"github.com/lanechi/gonex/g"
	"github.com/lanechi/gonex/ghttp"
	"github.com/lanechi/gonex/scheduler"
)

type HelloReq struct {
	g.Meta `       path:"/hello" method:"get" tags:"Hello" summary:"Say hello"`
	Name   string `                                                            query:"name"`
}

type HelloRes struct {
	Message string `json:"message"`
}

type HelloNamesReq struct {
	g.Meta `path:"/names" method:"get" summary:"List names"`
}

type HelloNamesRes []string

type HelloController struct{}

func (*HelloController) Hello(_ context.Context, req *HelloReq) (*HelloRes, error) {
	name := req.Name
	if name == "" {
		name = "gonex"
	}
	return &HelloRes{Message: "Hello, " + name + "!"}, nil
}

func (*HelloController) Names(context.Context, *HelloNamesReq) (HelloNamesRes, error) {
	return HelloNamesRes{"gonex", "gopher"}, nil
}

func main() {
	server := ghttp.NewServer(ghttp.WithMode(ghttp.DebugMode))
	if err := registerBackgroundJobs(server); err != nil {
		panic(err)
	}

	server.Group("/", func(group *ghttp.RouterGroup) {
		if err := group.Bind(&HelloController{}); err != nil {
			panic(err)
		}
	})
	if err := server.Run(); err != nil {
		panic(err)
	}
}

func registerBackgroundJobs(server *ghttp.Server) error {
	return server.Scheduler().Add(scheduler.Job{
		Name:     "basic-example-heartbeat",
		Schedule: scheduler.Every{Duration: time.Hour},
		Handler: func(ctx context.Context) error {
			server.Logger().Info(ctx, "basic scheduler heartbeat")
			return nil
		},
	})
}

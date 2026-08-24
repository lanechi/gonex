package main

import (
	"log"
	"strings"

	"github.com/lanechi/gonex/config"
	"github.com/lanechi/gonex/examples/template-demo/internal/controller"
	"github.com/lanechi/gonex/g"
	"github.com/lanechi/gonex/ghttp"
)

func main() {
	if err := config.Init(); err != nil {
		log.Fatal(err)
	}
	server := g.Server()
	if err := server.AddTemplateFunc("upper", strings.ToUpper); err != nil {
		log.Fatal(err)
	}
	server.Group("/", func(group *ghttp.RouterGroup) {
		if err := group.Bind(controller.NewPage()); err != nil {
			log.Fatal(err)
		}
	})
	if err := server.Run(); err != nil {
		log.Fatal(err)
	}
}

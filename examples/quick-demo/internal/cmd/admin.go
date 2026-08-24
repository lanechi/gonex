package cmd

import (
	"context"

	"github.com/lanechi/gonex/config"
	"github.com/lanechi/gonex/examples/quick-demo/internal/controller/hello"
	"github.com/lanechi/gonex/examples/quick-demo/internal/database"
	_ "github.com/lanechi/gonex/examples/quick-demo/internal/logic"
	"github.com/lanechi/gonex/g"
	"github.com/lanechi/gonex/ghttp"
	"github.com/spf13/cobra"
)

var serveAdminCmd = &cobra.Command{
	Use:     "admin",
	Aliases: []string{"serve-admin", "serve2"},
	Short:   "start admin http server on :8001",
	RunE: func(command *cobra.Command, _ []string) error {
		configuration := config.Default()
		if err := config.Init(); err != nil {
			return err
		}

		server := g.Server("admin")
		if err := server.Err(); err != nil {
			return err
		}

		if err := database.Initialize(configuration); err != nil {
			return err
		}
		server.Group("/admin", func(group *ghttp.RouterGroup) {
			if err := group.Bind(hello.NewV1()); err != nil {
				panic(err)
			}
		})
		server.OnStop(func(_ context.Context) error {
			return database.Close()
		})
		return server.Run(":8001")
	},
}

func init() {
	rootCmd.AddCommand(serveAdminCmd)
}

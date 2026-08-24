package cmd

import (
	"context"

	"github.com/lanechi/gonex/config"
	"github.com/lanechi/gonex/examples/quick-demo/internal/controller/hello"
	"github.com/lanechi/gonex/examples/quick-demo/internal/controller/user"
	"github.com/lanechi/gonex/examples/quick-demo/internal/database"
	_ "github.com/lanechi/gonex/examples/quick-demo/internal/logic"
	"github.com/lanechi/gonex/g"
	"github.com/lanechi/gonex/ghttp"
	"github.com/lanechi/gonex/logging"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "start http server",
	RunE: func(command *cobra.Command, _ []string) error {
		configuration := config.Default()
		if err := config.Init(); err != nil {
			return err
		}

		server := g.Server()

		if err := server.Err(); err != nil {
			return err
		}

		if err := database.Initialize(configuration); err != nil {
			return err
		}
		server.Group("/", func(group *ghttp.RouterGroup) {
			if err := group.Bind(user.NewV1()); err != nil {
				panic(err)
			}
		})
		server.Group("/hello", func(group *ghttp.RouterGroup) {
			if err := group.Bind(hello.NewV1()); err != nil {
				panic(err)
			}
		})
		server.OnStop(func(_ context.Context) error {
			return database.Close()
		})
		server.Logger().Info(
			command.Context(),
			"starting server",
			logging.String("server", server.Name()),
		)
		return server.Run()
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

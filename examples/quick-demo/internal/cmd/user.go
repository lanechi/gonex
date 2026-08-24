package cmd

import (
	"context"

	"github.com/lanechi/gonex/config"
	"github.com/lanechi/gonex/examples/quick-demo/internal/controller/hello"
	"github.com/lanechi/gonex/examples/quick-demo/internal/database"
	_ "github.com/lanechi/gonex/examples/quick-demo/internal/logic"
	"github.com/lanechi/gonex/ghttp"
	"github.com/spf13/cobra"
)

var userCmd = &cobra.Command{
	Use:     "user",
	Aliases: []string{"admin-with-address", "serve3"},
	Short:   "start user http server on :8002",
	RunE: func(command *cobra.Command, _ []string) error {
		configuration := config.Default()
		if err := config.Init(); err != nil {
			return err
		}

		server := ghttp.NewServer(
			ghttp.WithName("user"),
			ghttp.WithAddress(":8002"),
		)
		if err := server.Err(); err != nil {
			return err
		}
		if err := database.Initialize(configuration); err != nil {
			return err
		}
		server.Group("/user", func(group *ghttp.RouterGroup) {
			if err := group.Bind(hello.NewV1()); err != nil {
				panic(err)
			}
		})
		server.OnStop(func(_ context.Context) error {
			return database.Close()
		})
		return server.Run()
	},
}

func init() {
	rootCmd.AddCommand(userCmd)
}

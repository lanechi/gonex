package cmd

import (
	"context"

	"github.com/lanechi/gonex/config"
	"github.com/lanechi/gonex/examples/demo/internal/bootstrap/db"
	"github.com/lanechi/gonex/examples/demo/internal/controller/hello"
	_ "github.com/lanechi/gonex/examples/demo/internal/logic"
	"github.com/lanechi/gonex/ghttp"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use: "serve", Short: "start HTTP server",
	RunE: func(_ *cobra.Command, _ []string) error {
		if err := config.Init(); err != nil {
			return err
		}
		if err := db.InitializePostgres(config.Default()); err != nil {
			return err
		}
		server := ghttp.NewServer()
		if err := server.Err(); err != nil {
			_ = db.ClosePostgres()
			return err
		}
		if err := server.Bind(hello.NewV1()); err != nil {
			_ = db.ClosePostgres()
			return err
		}
		server.OnStop(func(context.Context) error { return db.ClosePostgres() })
		return server.Run()
	},
}

func init() { rootCmd.AddCommand(serveCmd) }

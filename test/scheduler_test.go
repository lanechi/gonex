package ghttp_test

import (
	"testing"

	"github.com/lanechi/gonex/config"
	"github.com/lanechi/gonex/ghttp"
)

func TestSchedulerConfiguration(t *testing.T) {
	disabled := config.New()
	disabled.Set("server.scheduler.enabled", false)
	server := ghttp.NewServer(ghttp.WithConfig(disabled))
	if err := server.Err(); err != nil {
		t.Fatal(err)
	}
	if server.Scheduler() == nil {
		t.Fatal("disabled scheduler is not available for registration")
	}

	invalidTimezone := config.New()
	invalidTimezone.Set("server.scheduler.timezone", "not/a-timezone")
	if err := ghttp.NewServer(ghttp.WithConfig(invalidTimezone)).Err(); err == nil {
		t.Fatal("invalid scheduler timezone did not surface through Server.Err")
	}
}

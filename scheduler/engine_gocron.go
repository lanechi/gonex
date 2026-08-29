package scheduler

import (
	"fmt"
	"strings"

	"github.com/go-co-op/gocron/v2"
)

func scheduleDefinition(schedule Schedule) (gocron.JobDefinition, error) {
	switch value := schedule.(type) {
	case Cron:
		return gocron.CronJob(value.Expr, cronHasSeconds(value.Expr)), nil
	case Every:
		return gocron.DurationJob(value.Duration), nil
	case Once:
		return gocron.OneTimeJob(gocron.OneTimeJobStartDateTime(value.At)), nil
	default:
		return nil, fmt.Errorf("unsupported scheduler schedule %T", schedule)
	}
}

func cronHasSeconds(expression string) bool {
	fields := strings.Fields(expression)
	if len(fields) > 0 && (strings.HasPrefix(fields[0], "TZ=") || strings.HasPrefix(fields[0], "CRON_TZ=")) {
		fields = fields[1:]
	}
	return len(fields) == 6
}

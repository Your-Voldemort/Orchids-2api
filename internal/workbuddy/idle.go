package workbuddy

import (
	"context"
	"io"
	"time"

	"orchids-api/internal/util"
)

// monitorStreamIdle is the WorkBuddy spelling of the shared idle monitor. The
// label is the only per-channel difference: it is what an operator reads in the
// timeout error.
func monitorStreamIdle(body io.ReadCloser, idle time.Duration, cancel context.CancelFunc) io.ReadCloser {
	return util.MonitorReadIdle(body, idle, cancel, "workbuddy")
}

// errStreamIdleTimeout is the timeout error the shared monitor raises for this
// channel's label, so a caller can recognise it by identity.
var errStreamIdleTimeout = util.ErrStreamIdle("workbuddy")

package main

import (
	"context"
	"strings"
	"time"
)

// Keep bounded subprocess diagnostics private; expose only known static codes.
type poolRefreshErrorBuffer struct{ data []byte }

func (b *poolRefreshErrorBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := 4096 - len(b.data)
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		b.data = append(b.data, p...)
	}
	return n, nil
}

func poolRefreshCause(ctx context.Context, err error, stderr string) string {
	if err == nil {
		return ""
	}
	if ctx.Err() == context.DeadlineExceeded {
		return "pool_refresh_timeout"
	}
	if ctx.Err() == context.Canceled {
		return "pool_refresh_cancelled"
	}
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		switch line {
		case "Error: pool_source_unavailable":
			return "pool_source_unavailable"
		case "Error: another_build_is_running":
			return "pool_refresh_busy"
		case "Error: no_candidates_matching_filters":
			return "pool_source_no_matching_candidates"
		case "Error: mirror_index_invalid", "Error: mirror_index_unavailable", "Error: mirror_time_invalid", "Error: mirror_snapshot_expired",
			"Error: mirror_snapshot_rollback", "Error: mirror_snapshot_conflict", "Error: mirror_download_failed",
			"Error: mirror_integrity_failed", "Error: mirror_csv_invalid":
			return strings.TrimPrefix(line, "Error: ")
		case "Error: invalid_input_or_filesystem_error", "Error: https_url_required", "Error: invalid_schedule_configuration", "Error: invalid_schedule_state":
			return "pool_refresh_input_error"
		}
		if strings.HasPrefix(line, "Error: invalid_setting_") {
			return "pool_refresh_input_error"
		}
	}
	return "pool_refresh_failed"
}

// Caller holds refreshMu. Wall time is displayed as elapsed time, not a guessed
// percentage of a network download whose size and response time are unknown.
func (c *Console) poolRefreshViewLocked() map[string]any {
	elapsed := 0
	started := ""
	if !c.refreshStarted.IsZero() {
		started = c.refreshStarted.UTC().Format(time.RFC3339Nano)
		end := time.Now()
		if t, err := time.Parse(time.RFC3339, c.refreshAt); err == nil {
			end = t
		}
		elapsed = max(0, int(end.Sub(c.refreshStarted).Seconds()))
	}
	return map[string]any{"status": c.refreshState, "started_at": started,
		"checked_at": c.refreshAt, "error": c.refreshError, "cause": c.refreshCause,
		"elapsed_seconds": elapsed, "timeout_seconds": 90}
}

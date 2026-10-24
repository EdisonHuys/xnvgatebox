package main

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPoolRefreshCauseNeverExposesRawProcessData(t *testing.T) {
	err := errors.New("private command or secret")
	for _, tc := range []struct{ stderr, want string }{
		{"Error: pool_source_unavailable\nprivate URL", "pool_source_unavailable"},
		{"Error: another_build_is_running", "pool_refresh_busy"},
		{"Error: invalid_setting_private_value", "pool_refresh_input_error"},
		{"Error: invalid_input_or_filesystem_error", "pool_refresh_input_error"},
		{"Traceback: private password", "pool_refresh_failed"},
	} {
		if got := poolRefreshCause(context.Background(), err, tc.stderr); got != tc.want {
			t.Fatalf("unsafe or incorrect cause: %q", got)
		}
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if poolRefreshCause(ctx, err, "") != "pool_refresh_timeout" {
		t.Fatal("timeout not distinguished")
	}
	var out poolRefreshErrorBuffer
	data := []byte(strings.Repeat("private", 10000))
	if n, err := out.Write(data); err != nil || n != len(data) || len(out.data) != 4096 {
		t.Fatal("unbounded diagnostics or short subprocess write")
	}
}

func TestConsoleRefreshStartsImmediatelyAndReportsSafeFailure(t *testing.T) {
	c := consoleFixture(t)
	usePanel(t, &fakePanel{})
	dir := t.TempDir()
	script := filepath.Join(dir, "source.py")
	if err := os.WriteFile(script, []byte("import sys,time\ntime.sleep(0.2)\nsys.stderr.write('Error: pool_source_unavailable\\nprivate-value\\n')\nsys.exit(2)\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FANOUT_TOOLBOX_SCRIPT", script)
	w := httptest.NewRecorder()
	c.apiPoolRefresh(w, httptest.NewRequest("POST", "/", nil))
	if w.Code != 202 {
		t.Fatal(w.Code)
	}
	c.refreshMu.Lock()
	started, finished := c.refreshStarted, c.refreshDone
	view := c.poolRefreshViewLocked()
	c.refreshMu.Unlock()
	if started.IsZero() || view["status"] != "running" || view["timeout_seconds"] != 90 {
		t.Fatal("missing immediate progress")
	}
	c.apiPoolRefresh(httptest.NewRecorder(), httptest.NewRequest("POST", "/", nil))
	c.refreshMu.Lock()
	unchanged := started.Equal(c.refreshStarted) && finished == c.refreshDone
	c.refreshMu.Unlock()
	if !unchanged {
		t.Fatal("duplicate refresh replaced the active task")
	}
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("refresh did not finish")
	}
	w = httptest.NewRecorder()
	c.apiConsole(w, httptest.NewRequest("GET", "/", nil))
	if !strings.Contains(w.Body.String(), `"cause":"pool_source_unavailable"`) || strings.Contains(w.Body.String(), "private-value") {
		t.Fatal("cause hidden or private data exposed")
	}
}

package main

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func updateFixture(t *testing.T) (*UpdateManager, *releaseInfo) {
	t.Helper()
	r := &releaseInfo{}
	if err := json.Unmarshal([]byte(`{"tag_name":"v99.0.0","assets":[{"name":"xnvgatebox-linux-`+assetArch()+`.tar.gz"},{"name":"checksums.txt"}]}`), r); err != nil {
		t.Fatal(err)
	}
	u := newUpdateManager(t.TempDir())
	u.statusPath = filepath.Join(t.TempDir(), "private", "status.json")
	u.latest = func() (*releaseInfo, error) { return r, nil }
	u.ready = func() error { return nil }
	u.alive = func(UpdateJob) bool { return true }
	u.launch = func(string, string) error { return nil }
	return u, r
}

func TestUpdateApplyConcurrentCreatesOneIndependentJob(t *testing.T) {
	u, _ := updateFixture(t)
	launches := 0
	u.launch = func(tag, id string) error {
		launches++
		if tag != "v99.0.0" || len(id) != 24 {
			t.Errorf("unexpected update identity")
		}
		return nil
	}
	var wg sync.WaitGroup
	codes := make(chan int, 12)
	for n := 0; n < 12; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			u.apply(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"expected_version":"v99.0.0"}`)))
			codes <- w.Code
		}()
	}
	wg.Wait()
	close(codes)
	accepted := 0
	for c := range codes {
		if c == 202 {
			accepted++
		} else if c != 409 {
			t.Fatalf("HTTP %d", c)
		}
	}
	if accepted != 1 || launches != 1 {
		t.Fatalf("accepted=%d launches=%d", accepted, launches)
	}
	j, err := u.readJob()
	if err != nil || j.Unit != "xnvgatebox-update" || j.Status != "running" {
		t.Fatalf("job=%+v err=%v", j, err)
	}
	s, _ := os.Stat(u.statusPath)
	if s.Mode().Perm() != 0600 {
		t.Fatal("job permissions")
	}
}

func TestUpdateRejectsChangedVersionBadReleaseAndUnsupportedInstance(t *testing.T) {
	for _, c := range []string{"changed", "asset", "draft", "prerelease", "tag", "instance", "lookup"} {
		t.Run(c, func(t *testing.T) {
			u, r := updateFixture(t)
			launches := 0
			u.launch = func(string, string) error { launches++; return nil }
			expected := "v99.0.0"
			switch c {
			case "changed":
				expected = "v98.0.0"
			case "asset":
				r.Assets = nil
			case "draft":
				r.Draft = true
			case "prerelease":
				r.Prerelease = true
			case "tag":
				r.TagName = "v99.0.0;injection"
			case "instance":
				u.ready = func() error { return errors.New("unsupported instance") }
			case "lookup":
				u.latest = func() (*releaseInfo, error) { return nil, errors.New("secret-bearing upstream error") }
			}
			w := httptest.NewRecorder()
			u.apply(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"expected_version":"`+expected+`"}`)))
			if w.Code != 409 && w.Code != 502 {
				t.Fatalf("HTTP %d", w.Code)
			}
			if launches != 0 || strings.Contains(w.Body.String(), "secret-bearing") {
				t.Fatal("unsafe update")
			}
		})
	}
}

func TestUpdateNoDowngradeOrSameVersionLaunch(t *testing.T) {
	previous := version
	version = "v99.0.0"
	defer func() { version = previous }()
	u, _ := updateFixture(t)
	u.launch = func(string, string) error { t.Fatal("same version launched"); return nil }
	w := httptest.NewRecorder()
	u.apply(w, httptest.NewRequest("POST", "/", nil))
	if w.Code != 200 {
		t.Fatalf("HTTP %d", w.Code)
	}
}

func TestUpdateLaunchFailurePersistsSafeStatus(t *testing.T) {
	u, _ := updateFixture(t)
	u.launch = func(string, string) error { return errors.New("private-command-details") }
	w := httptest.NewRecorder()
	u.apply(w, httptest.NewRequest("POST", "/", nil))
	j, err := u.readJob()
	if w.Code != 500 || err != nil || j.Status != "failed" || strings.Contains(w.Body.String(), "private-command-details") {
		t.Fatal("launch failure handling")
	}
}

func TestUpdateStatusWhitelistsPrivateFieldsAndDoesNotRewriteWorkerFile(t *testing.T) {
	u, _ := updateFixture(t)
	u.alive = func(UpdateJob) bool { return false }
	j := UpdateJob{ID: "test", Status: "running", UpdatedAt: float64(time.Now().Add(-time.Minute).Unix()), PID: os.Getpid(), ProcessStart: "private-start", BootID: "private-boot", Unit: "xnvgatebox-update"}
	blob, _ := json.Marshal(j)
	os.MkdirAll(filepath.Dir(u.statusPath), 0700)
	os.WriteFile(u.statusPath, blob, 0600)
	w := httptest.NewRecorder()
	u.status(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "interrupted") {
		t.Fatal("dead worker not reported")
	}
	for _, private := range []string{"private-start", "private-boot", `"pid"`, `"unit"`} {
		if strings.Contains(w.Body.String(), private) {
			t.Fatal("private status exposed")
		}
	}
	after, _ := os.ReadFile(u.statusPath)
	if string(after) != string(blob) {
		t.Fatal("status reader modified worker record")
	}
}

func TestUpdateFingerprintRejectsPIDReuseAndOtherBoot(t *testing.T) {
	start, boot := processFingerprint(os.Getpid())
	if start == "" || boot == "" {
		t.Fatal("missing Linux process identity")
	}
	j := UpdateJob{PID: os.Getpid(), ProcessStart: start, BootID: boot}
	if !updateJobAlive(j) {
		t.Fatal("current process not recognized")
	}
	j.ProcessStart += "x"
	if updateJobAlive(j) {
		t.Fatal("PID reuse accepted")
	}
	j.ProcessStart = start
	j.BootID += "x"
	if updateJobAlive(j) {
		t.Fatal("different boot accepted")
	}
}

func TestUpdateMalformedJobAndInvalidLaunchArguments(t *testing.T) {
	u, _ := updateFixture(t)
	os.MkdirAll(filepath.Dir(u.statusPath), 0700)
	os.WriteFile(u.statusPath, []byte("broken"), 0600)
	w := httptest.NewRecorder()
	u.apply(w, httptest.NewRequest("POST", "/", nil))
	if w.Code != 500 {
		t.Fatal("corrupt job overwritten")
	}
	for _, tag := range []string{"v1.2.3/../bad", "v1.2.3-rc1", "v-1.2.3", "latest"} {
		if validReleaseTag(tag) || launchCompleteUpdate(tag, "bad") == nil {
			t.Fatal("invalid tag accepted")
		}
	}
}

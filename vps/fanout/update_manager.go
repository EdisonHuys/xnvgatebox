package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var releaseTagPattern = regexp.MustCompile(`^v[0-9]{1,9}\.[0-9]{1,9}\.[0-9]{1,9}$`)

func validReleaseTag(v string) bool { return releaseTagPattern.MatchString(v) }
func releaseHasPackage(r *releaseInfo) bool {
	want := "xnvgatebox-linux-" + assetArch() + ".tar.gz"
	pkg, sums := false, false
	for _, a := range r.Assets {
		if a.Name == want {
			pkg = true
		}
		if a.Name == "checksums.txt" {
			sums = true
		}
	}
	return pkg && sums
}

type UpdateJob struct {
	ID              string  `json:"id,omitempty"`
	Status          string  `json:"status"`
	Phase           string  `json:"phase,omitempty"`
	Target          string  `json:"target,omitempty"`
	Message         string  `json:"message"`
	StartedAt       float64 `json:"started_at,omitempty"`
	UpdatedAt       float64 `json:"updated_at,omitempty"`
	BackupAvailable bool    `json:"backup_available"`
	RolledBack      bool    `json:"rolled_back"`
	PID             int     `json:"pid,omitempty"`
	ProcessStart    string  `json:"process_start,omitempty"`
	BootID          string  `json:"boot_id,omitempty"`
	Unit            string  `json:"unit,omitempty"`
} // Explicit JSON field names are shared with the command-line installer.
func publicUpdateJob(j UpdateJob) map[string]any {
	return map[string]any{"id": j.ID, "status": j.Status, "phase": j.Phase, "target": j.Target, "message": j.Message, "started_at": j.StartedAt, "updated_at": j.UpdatedAt, "backup_available": j.BackupAvailable, "rolled_back": j.RolledBack}
}

type UpdateManager struct {
	mu                  sync.Mutex
	workDir, statusPath string
	latest              func() (*releaseInfo, error)
	launch              func(string, string) error
	alive               func(UpdateJob) bool
	ready               func() error
}

func newUpdateManager(dir string) *UpdateManager {
	u := &UpdateManager{workDir: dir, statusPath: "/var/lib/xnvgatebox-deploy/status.json", latest: fetchLatestRelease}
	u.ready = func() error {
		if panel, err := openPanel(); err != nil || panel.Kind() != "native" {
			return fmt.Errorf("请使用该实例对应的部署入口更新")
		}
		absolute, _ := filepath.Abs(dir)
		if filepath.Clean(absolute) != "/var/lib/xnvgatebox/fanout" {
			return fmt.Errorf("在线更新用于系统安装实例，请使用本实例的安装入口")
		}
		for _, path := range []string{"/usr/local/bin/xnvgatebox", "/usr/local/lib/xnvgatebox/deploy/installer.py"} {
			s, e := os.Stat(path)
			if e != nil || !s.Mode().IsRegular() {
				return fmt.Errorf("请先使用新版安装入口完成迁移")
			}
		}
		if _, err := exec.LookPath("systemd-run"); err != nil {
			return fmt.Errorf("当前系统缺少 systemd 更新任务支持")
		}
		return nil
	}
	u.launch = launchCompleteUpdate
	u.alive = updateJobAlive
	return u
}
func processFingerprint(pid int) (string, string) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return "", ""
	}
	end := strings.LastIndex(string(data), ")")
	if end < 0 {
		return "", ""
	}
	fields := strings.Fields(string(data)[end+1:])
	if len(fields) < 20 {
		return "", ""
	}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", ""
	}
	return fields[19], strings.TrimSpace(string(boot))
}
func updateJobAlive(j UpdateJob) bool {
	if j.PID > 0 {
		start, boot := processFingerprint(j.PID)
		if start != "" && start == j.ProcessStart && boot == j.BootID {
			return true
		}
	}
	if j.Unit == "xnvgatebox-update" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", j.Unit+".service").Run() == nil
	}
	return false
}
func (u *UpdateManager) readJob() (UpdateJob, error) {
	blob, err := os.ReadFile(u.statusPath)
	if os.IsNotExist(err) {
		return UpdateJob{Status: "idle", Message: "尚无更新任务"}, nil
	}
	if err != nil || len(blob) > 65536 {
		return UpdateJob{}, fmt.Errorf("无法读取更新状态")
	}
	var j UpdateJob
	if json.Unmarshal(blob, &j) != nil {
		return j, fmt.Errorf("更新状态记录损坏，保留原文件")
	}
	if j.Status == "running" && time.Since(time.Unix(int64(j.UpdatedAt), 0)) > 5*time.Second && !u.alive(j) {
		j.Status = "interrupted"
		j.Message = "更新任务已中断，请重新运行安装入口恢复或重试"
	}
	return j, nil
}
func (u *UpdateManager) status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(405)
		return
	}
	u.mu.Lock()
	j, err := u.readJob()
	u.mu.Unlock()
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, publicUpdateJob(j))
}
func (u *UpdateManager) check(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(405)
		return
	}
	rel, err := u.latest()
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": "检查更新失败，请稍后重试"})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	st := UpdateStatus{Current: version, Latest: rel.TagName, HasUpdate: versionLess(version, rel.TagName), Notes: rel.Body, URL: rel.HTMLURL, Supported: releaseHasPackage(rel)}
	writeJSON(w, 200, st)
}
func (u *UpdateManager) apply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	var in struct {
		Expected string `json:"expected_version"`
	}
	if r.ContentLength != 0 {
		if readConsoleJSON(w, r, &in) != nil {
			writeJSON(w, 400, map[string]string{"error": "请求格式无效"})
			return
		}
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	old, err := u.readJob()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if old.Status == "running" {
		writeJSON(w, 409, map[string]string{"error": "已有更新任务运行"})
		return
	}
	rel, err := u.latest()
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": "检查更新失败，请稍后重试"})
		return
	}
	if !validReleaseTag(rel.TagName) || rel.Draft || rel.Prerelease || !releaseHasPackage(rel) {
		writeJSON(w, 409, map[string]string{"error": "当前架构暂无可用的正式完整包"})
		return
	}
	if in.Expected != "" && in.Expected != rel.TagName {
		writeJSON(w, 409, map[string]string{"error": "最新版本已变化，请重新检查更新"})
		return
	}
	if !versionLess(version, rel.TagName) {
		writeJSON(w, 200, map[string]any{"ok": true, "restarting": false, "message": "已经是最新版"})
		return
	}
	if err := u.ready(); err != nil {
		writeJSON(w, 409, map[string]string{"error": err.Error()})
		return
	}
	id := randomHex(12)
	now := float64(time.Now().Unix())
	j := UpdateJob{ID: id, Status: "running", Phase: "preparing", Target: rel.TagName, Message: "正在准备完整包更新", StartedAt: now, UpdatedAt: now, Unit: "xnvgatebox-update"}
	if err := os.MkdirAll(filepath.Dir(u.statusPath), 0700); err != nil {
		writeJSON(w, 500, map[string]string{"error": "无法保存更新状态"})
		return
	}
	blob, _ := json.Marshal(j)
	if atomicConfigWrite(u.statusPath, blob) != nil {
		writeJSON(w, 500, map[string]string{"error": "无法保存更新状态"})
		return
	}
	if err := u.launch(rel.TagName, id); err != nil {
		j.Status, j.Phase, j.Message = "failed", "failed", "无法启动独立更新任务，请使用命令行更新"
		blob, _ := json.Marshal(j)
		_ = atomicConfigWrite(u.statusPath, blob)
		writeJSON(w, 500, map[string]string{"error": j.Message})
		return
	}
	writeJSON(w, 202, publicUpdateJob(j))
}
func launchCompleteUpdate(tag, id string) error {
	if !validReleaseTag(tag) || !regexp.MustCompile(`^[a-f0-9]{24}$`).MatchString(id) {
		return fmt.Errorf("invalid update identity")
	}
	logDir := filepath.Join("/var/backups/xnvgatebox", "update-"+id)
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	args := []string{"--unit=xnvgatebox-update", "--collect", "--quiet", "--property=Type=exec", "--property=UMask=0077", "--property=RuntimeMaxSec=900", "--property=StandardOutput=append:" + filepath.Join(logDir, "install.log"), "--property=StandardError=inherit", "--", "/usr/local/bin/xnvgatebox", "update", "--version", tag, "--job-id", id}
	return exec.CommandContext(ctx, "systemd-run", args...).Run()
}

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func (c *Console) shutdown() {
	c.cancel()
	// Wait for an in-flight quantity change before stopping user tunnels.
	c.targetMu.Lock()
	c.targetMu.Unlock()
	c.checksMu.Lock()
	job := c.check
	c.checksMu.Unlock()
	if job != nil && job.finished != nil {
		<-job.finished
	}
	c.refreshMu.Lock()
	finished := c.refreshDone
	c.refreshMu.Unlock()
	if finished != nil {
		<-finished
	}
}

func diagnosticConfigOwned(workDir, ns, executable, config string) bool {
	if filepath.Base(executable) != "openvpn" || filepath.Base(config) != ns+".ovpn" {
		return false
	}
	dir := filepath.Dir(config)
	return filepath.Dir(dir) == filepath.Clean(workDir) && strings.HasPrefix(filepath.Base(dir), ".connectivity-")
}

// Called only by the root service at startup, after acquiring the instance
// lock. SIGKILL cannot run defers, so recover only our reserved diagnostic
// namespace and private temporary directories before accepting new jobs.
func (c *Console) cleanupDiagnostics() error {
	for slot := diagnosticFirstSlot; slot < diagnosticFirstSlot+diagnosticWorkers; slot++ {
		if err := c.cleanupDiagnosticSlot(slot); err != nil {
			return err
		}
	}
	dirs, err := filepath.Glob(filepath.Join(c.m.workDir, ".connectivity-*"))
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("无法清理检测临时目录")
		}
	}
	return nil
}

func (c *Console) cleanupDiagnosticSlot(slot int) error {
	t := &Tunnel{Slot: slot}
	nsPath := filepath.Join("/run/netns", t.nsName())
	nsInfo, err := os.Stat(nsPath)
	if err == nil {
		out, err := cmdOutput(exec.Command("ip", "netns", "pids", t.nsName()))
		if err != nil {
			return fmt.Errorf("无法核对检测进程")
		}
		for _, text := range strings.Fields(string(out)) {
			pid, err := strconv.Atoi(text)
			if err != nil {
				return fmt.Errorf("检测进程编号异常")
			}
			fd, err := unix.PidfdOpen(pid, 0)
			if err == unix.ESRCH {
				continue
			}
			if err != nil {
				return fmt.Errorf("无法固定检测进程身份")
			}
			proc := filepath.Join("/proc", text)
			exe, e1 := os.Readlink(filepath.Join(proc, "exe"))
			blob, e2 := os.ReadFile(filepath.Join(proc, "cmdline"))
			info, e3 := os.Stat(filepath.Join(proc, "ns/net"))
			config := ""
			args := strings.Split(string(blob), "\x00")
			for i, arg := range args {
				if arg == "--config" && i+1 < len(args) {
					config = args[i+1]
					break
				}
			}
			if os.IsNotExist(e1) || os.IsNotExist(e2) || os.IsNotExist(e3) {
				unix.Close(fd)
				continue
			}
			if e1 != nil || e2 != nil || e3 != nil || !os.SameFile(nsInfo, info) || !diagnosticConfigOwned(c.m.workDir, t.nsName(), exe, config) {
				unix.Close(fd)
				return fmt.Errorf("检测命名空间存在无法确认归属的进程")
			}
			err = unix.PidfdSendSignal(fd, unix.SIGKILL, nil, 0)
			unix.Close(fd)
			if err != nil && err != unix.ESRCH {
				return fmt.Errorf("无法停止残留检测进程")
			}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("无法核对检测命名空间")
	}
	t.teardownNetns()
	_ = os.RemoveAll(filepath.Join("/etc/netns", t.nsName()))
	return nil
}

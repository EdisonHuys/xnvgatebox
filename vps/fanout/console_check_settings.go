package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

type checkSettings struct {
	Parallelism int `json:"parallelism"`
}

func validateCheckParallelism(n int) error {
	if n < 1 || n > networkLinkCount {
		return fmt.Errorf("检测并发须为正整数，不能超过实例 /30 地址容量 %d", networkLinkCount)
	}
	return nil
}

func (c *Console) loadCheckSettings() error {
	c.checkParallelism = defaultCheckParallelism
	c.m.connectBudget.setLimit(defaultCheckParallelism)
	blob, err := os.ReadFile(filepath.Join(c.m.workDir, "check-settings.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var s checkSettings
	if json.Unmarshal(blob, &s) != nil || validateCheckParallelism(s.Parallelism) != nil {
		return fmt.Errorf("检测设置损坏，保留原文件")
	}
	c.checkParallelism = s.Parallelism
	c.m.connectBudget.setLimit(s.Parallelism)
	return nil
}

// Caller holds checksMu; atomic persistence precedes publication.
func (c *Console) saveCheckSettings(n int) error {
	if err := validateCheckParallelism(n); err != nil {
		return err
	}
	blob, _ := json.Marshal(checkSettings{Parallelism: n})
	if err := atomicConfigWrite(filepath.Join(c.m.workDir, "check-settings.json"), blob); err != nil {
		return fmt.Errorf("检测设置保存失败")
	}
	c.checkParallelism = n
	c.m.connectBudget.setLimit(n)
	return nil
}

func (c *Console) apiCheckSettings(w http.ResponseWriter, r *http.Request) {
	var in checkSettings
	if err := readConsoleJSON(w, r, &in); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	c.checksMu.Lock()
	defer c.checksMu.Unlock()
	if c.ctx.Err() != nil {
		writeJSON(w, 503, map[string]string{"error": "管理服务正在停止"})
		return
	}
	if err := c.saveCheckSettings(in.Parallelism); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, in)
}

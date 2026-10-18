package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const updateRepo = "hanhaoops/xnvgatebox"

// releaseInfo 是 GitHub Releases API 里我们关心的字段。
type releaseInfo struct {
	TagName    string `json:"tag_name"`
	Name       string `json:"name"`
	Body       string `json:"body"`
	HTMLURL    string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// UpdateStatus 回给界面：当前版本、最新版本、有没有新版、更新内容。
type UpdateStatus struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	HasUpdate bool   `json:"has_update"`
	Notes     string `json:"notes"`
	URL       string `json:"url"`
	Supported bool   `json:"supported"`
}

// goarch 把 runtime.GOARCH 映射成 release 资产用的名字。
func assetArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	default:
		return runtime.GOARCH
	}
}

// fetchLatestRelease 拉取最新 release 元数据。
func fetchLatestRelease() (*releaseInfo, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", updateRepo)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "xnvgatebox-updater")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub 返回 HTTP %d", resp.StatusCode)
	}
	var rel releaseInfo
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel); err != nil {
		return nil, err
	}
	if !validReleaseTag(rel.TagName) || rel.Draft || rel.Prerelease {
		return nil, fmt.Errorf("正式发布信息无效")
	}
	return &rel, nil
}

// checkUpdate 比对当前版本与最新 release。
func checkUpdate() (*UpdateStatus, error) {
	rel, err := fetchLatestRelease()
	if err != nil {
		return nil, err
	}
	cur := strings.TrimSpace(version)
	latest := strings.TrimSpace(rel.TagName)
	st := &UpdateStatus{
		Current:   cur,
		Latest:    latest,
		Notes:     strings.TrimSpace(rel.Body),
		URL:       rel.HTMLURL,
		HasUpdate: versionLess(cur, latest),
		Supported: releaseHasPackage(rel),
	}
	return st, nil
}

// versionLess 判断 cur 是否比 latest 旧。解析 vX.Y.Z 做数值比较；
// dev 或无法解析时保守认为"有更新"（让用户能装上正式版）。
func versionLess(cur, latest string) bool {
	if latest == "" {
		return false
	}
	if cur == "" || cur == "dev" {
		return true
	}
	cn, cok := parseSemver(cur)
	ln, lok := parseSemver(latest)
	if !cok || !lok {
		return cur != latest
	}
	for i := 0; i < 3; i++ {
		if cn[i] != ln[i] {
			return cn[i] < ln[i]
		}
	}
	return false
}

func parseSemver(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	// 去掉预发布/构建后缀
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return out, false
	}
	for i := 0; i < len(parts); i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

func sha256FromList(listPath, name string) (string, error) {
	blob, err := os.ReadFile(listPath)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(blob), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("校验和列表里没有 %s", name)
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// extractBinary 从 tar.gz 里取出指定文件名的成员写到 dst。
func extractBinary(tarGz, member, dst string) error {
	f, err := os.Open(tarGz)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hd, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("包里没有 %s", member)
		}
		if err != nil {
			return err
		}
		if filepath.Base(hd.Name) != member {
			continue
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			return err
		}
		defer out.Close()
		if _, err := io.Copy(out, tr); err != nil {
			return err
		}
		return nil
	}
}

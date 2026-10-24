package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConsoleSourceSettingsAreOptInAndPreservePrivateFields(t *testing.T) {
	c := consoleFixture(t)
	path := filepath.Join(t.TempDir(), "pool.json")
	before := `{"source_urls":["https://private.example/?key=SYNTHETIC_PRIVATE"],"countries":["JP"],"sstp_concurrency":17,"mirror_fallback":{"enabled":false,"max_age_hours":24,"private_note":"SYNTHETIC_PRIVATE"}}`
	if err := os.WriteFile(path, []byte(before), 0600); err != nil { t.Fatal(err) }
	t.Setenv("FANOUT_POOL_CONFIG", path)
	w := httptest.NewRecorder()
	c.apiSourceSettings(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 || strings.Contains(w.Body.String(), "SYNTHETIC_PRIVATE") || !strings.Contains(w.Body.String(), `"mirror_enabled":false`) {
		t.Fatal("default changed or private settings exposed")
	}
	w = httptest.NewRecorder()
	c.apiSourceSettings(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"mirror_enabled":true,"source_timeout_seconds":8,"mirror_max_age_hours":24}`)))
	if w.Code != 200 { t.Fatal(w.Code, w.Body.String()) }
	blob, _ := os.ReadFile(path)
	var fields map[string]json.RawMessage
	if json.Unmarshal(blob, &fields) != nil { t.Fatal("invalid persisted JSON") }
	if !strings.Contains(string(fields["source_urls"]), "SYNTHETIC_PRIVATE") || !strings.Contains(string(fields["mirror_fallback"]), "private_note") || string(fields["sstp_concurrency"]) != "17" || !strings.Contains(string(fields["countries"]), "JP") {
		t.Fatal("unrelated private configuration changed")
	}
	settings, _, err := readSourceOptions(path)
	if err != nil || !settings.MirrorEnabled || settings.SourceTimeout != 8 || settings.MirrorMaxAgeHours != 24 {
		t.Fatal("source preferences not persisted")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 { t.Fatal("configuration became public") }
	for _, body := range []string{`{}`, `{"mirror_enabled":null}`, `{"mirror_enabled":true,"source_timeout_seconds":100,"mirror_max_age_hours":24}`, `{"mirror_enabled":false,"source_timeout_seconds":8,"mirror_max_age_hours":0}`} {
		w = httptest.NewRecorder()
		c.apiSourceSettings(w, httptest.NewRequest("POST", "/", strings.NewReader(body)))
		after, _ := os.ReadFile(path)
		if w.Code != 400 || string(after) != string(blob) { t.Fatal("invalid settings replaced configuration") }
	}
}

func TestPoolSourceViewSeparatesSnapshotAndLocalRetrieval(t *testing.T) {
	now := time.Now().UTC()
	s := poolSourceMetadata{Kind: "mirror", Retrieval: "local_builder", RetrievedAt: now.Format(time.RFC3339), MirrorProvider: poolMirrorProvider,
		SnapshotFetchedAt: now.Add(-4*time.Hour).Format(time.RFC3339), DataCommit: strings.Repeat("a",40), IntegrityVerified: true}
	v := publicPoolSource(s)
	if v.Kind != "mirror" || !v.Stale || !v.IntegrityVerified || v.SnapshotFetchedAt == v.RetrievedAt || !strings.Contains(v.Label,"本 VPS") {
		t.Fatal("mirror age or local execution is ambiguous")
	}
	s.MirrorProvider = "SYNTHETIC_PRIVATE"
	blob, _ := json.Marshal(publicPoolSource(s))
	if strings.Contains(string(blob), "SYNTHETIC_PRIVATE") || strings.Contains(string(blob), `"integrity_verified":true`) { t.Fatal("unrecognized publisher trusted or exposed") }
	s.MirrorProvider = poolMirrorProvider; s.DataCommit="bad-commit"
	if publicPoolSource(s).IntegrityVerified { t.Fatal("invalid metadata marked checked") }
	s.Retrieval="external_worker"
	if publicPoolSource(s).Kind != "legacy" { t.Fatal("external observations represented as local retrieval") }
}

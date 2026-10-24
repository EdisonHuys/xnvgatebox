package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAutomationSettingsMinimumPersistAndRestart(t *testing.T) {
	c:=consoleFixture(t)
	w:=httptest.NewRecorder();c.apiAutomation(w,httptest.NewRequest("GET","/",nil))
	if w.Code!=200 || !strings.Contains(w.Body.String(),`"refresh_minutes":60`) || !strings.Contains(w.Body.String(),`"check_minutes":60`) {t.Fatal("wrong defaults")}
	path:=filepath.Join(c.m.workDir,"automation.json");before,_:=os.ReadFile(path)
	for _,body:=range []string{`{}`,`{"refresh_enabled":true,"refresh_minutes":29,"check_enabled":true,"check_minutes":60}`,`{"refresh_enabled":true,"refresh_minutes":60,"check_enabled":false,"check_minutes":0}`,`{"refresh_enabled":null,"refresh_minutes":60,"check_enabled":true,"check_minutes":60}`} {
		w=httptest.NewRecorder();c.apiAutomation(w,httptest.NewRequest("POST","/",strings.NewReader(body)))
		after,_:=os.ReadFile(path);if w.Code!=400 || string(after)!=string(before) {t.Fatal("invalid interval changed settings")}
	}
	w=httptest.NewRecorder();c.apiAutomation(w,httptest.NewRequest("POST","/",strings.NewReader(`{"refresh_enabled":false,"refresh_minutes":30,"check_enabled":false,"check_minutes":120}`)))
	if w.Code!=200 {t.Fatal(w.Code)}
	reloaded,err:=newConsole(NewManager(maximumSlots,c.m.workDir));if err!=nil {t.Fatal(err)};defer reloaded.cancel()
	if reloaded.automation.settings!= (automationSettings{false,30,false,120}) {t.Fatal("restart reset user's schedule")}
	info,_:=os.Stat(path);if info.Mode().Perm()!=0600 {t.Fatal("schedule file became public")}
}

func TestAutomationDoesNotOverlapManualCheckAndWaitsAfterCompletion(t *testing.T) {
	c:=defaultTargetFixture(t);c.m.nodes=[]Node{{HostName:"candidate",Config:"fixture"}}
	c.automation.mu.Lock();c.automation.settings.RefreshEnabled=false;c.automation.check.AnchorAt=time.Now().Add(-2*time.Hour);c.automation.mu.Unlock()
	release:=make(chan struct{});var calls atomic.Int32
	c.probe=func(ctx context.Context,n Node,slot int,progress func(string))CheckResult {calls.Add(1);select{case <-release:case <-ctx.Done():};return CheckResult{Host:n.HostName,Status:"passed"}}
	j,err:=c.startCheck(CheckRequest{All:true});if err!=nil{t.Fatal(err)}
	c.automationTick(time.Now());c.automationTick(time.Now())
	if c.check!=j {t.Fatal("automatic task replaced manual work")}
	close(release);select{case <-j.finished:case <-time.After(3*time.Second):t.Fatal("check stuck")}
	c.automationTick(time.Now())
	if calls.Load()!=1 {t.Fatal("back-to-back batch ignored completion interval")}
	c.automation.mu.Lock();state:=c.automation.check;c.automation.mu.Unlock()
	if state.Status!="completed" || state.Passed!=1 || runDue(state,60,time.Now().Add(59*time.Minute)) {t.Fatal("completion time not persisted")}
}

func TestAutomationRefreshPrecedesCheckAndFailedSourceKeepsOldCandidates(t *testing.T) {
	c:=defaultTargetFixture(t);c.m.nodes=[]Node{{HostName:"retained",Config:"fixture"}}
	c.automation.mu.Lock();c.automation.check.AnchorAt=time.Now().Add(-2*time.Hour);c.automation.mu.Unlock()
	dir:=t.TempDir();t.Setenv("FANOUT_VALIDATED_POOL_FILE",filepath.Join(dir,"pool.json"))
	script:=filepath.Join(dir,"source.py")
	code:="import sys,json,datetime,pathlib\np=pathlib.Path(sys.argv[sys.argv.index('--schedule-dir')+1])\nt=datetime.datetime.now(datetime.timezone.utc).isoformat()\n(p/'refresh-state.json').write_text(json.dumps({'status':'failed','started_at':t,'finished_at':t,'nodes':0}))\nprint(json.dumps({'status':'source_refresh_skipped'}))\n"
	if err:=os.WriteFile(script,[]byte(code),0600);err!=nil {t.Fatal(err)};t.Setenv("FANOUT_TOOLBOX_SCRIPT",script)
	c.probe=func(ctx context.Context,n Node,slot int,progress func(string))CheckResult{return CheckResult{Host:n.HostName,Status:"passed"}}
	c.automationTick(time.Now())
	c.refreshMu.Lock();done:=c.refreshDone;c.refreshMu.Unlock()
	if done==nil || c.check!=nil {t.Fatal("detection started before refresh")}
	select{case <-done:case <-time.After(3*time.Second):t.Fatal("refresh stuck")}
	c.automationTick(time.Now())
	c.checksMu.Lock();j:=c.check;c.checksMu.Unlock();if j==nil {t.Fatal("retained candidates not checked after source failure")}
	select{case <-j.finished:case <-time.After(3*time.Second):t.Fatal("retained check stuck")}
	if len(c.m.Tunnels())!=0 {t.Fatal("test unexpectedly created live tunnels")}
}

func TestAutomationInterruptedCheckKeepsIntervalAndNoCandidateBackoff(t *testing.T) {
	c:=defaultTargetFixture(t)
	started:=time.Now().Add(-10*time.Minute).UTC()
	blob,_:=json.Marshal(automationRun{Status:"running",StartedAt:started})
	if err:=os.WriteFile(filepath.Join(c.m.workDir,"check-schedule-state.json"),blob,0600);err!=nil {t.Fatal(err)}
	if err:=c.loadAutomation();err!=nil {t.Fatal(err)}
	if c.automation.check.Status!="interrupted" || runDue(c.automation.check,60,time.Now()) {t.Fatal("restart retriggered an interrupted batch too early")}
	c.automation.mu.Lock();c.automation.settings.RefreshEnabled=false;c.automation.check.StartedAt=time.Now().Add(-2*time.Hour);c.automation.mu.Unlock()
	c.automationTick(time.Now())
	if c.automation.check.Status!="no_candidates" || runDue(c.automation.check,60,time.Now()) {t.Fatal("empty pool spun automatic checks")}
}

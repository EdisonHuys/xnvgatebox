package main

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func clashRealityFixture(name string) string {
	return "vless://fbaea75a-e91c-4ad9-9410-7e43aa967766@203.0.113.1:443?type=tcp&security=reality&flow=xtls-rprx-vision&sni=example.com&fp=chrome&pbk="+base64.RawURLEncoding.EncodeToString(make([]byte,32))+"&sid=12345678#"+url.PathEscape(name)
}

func TestClashProfilePreservesRealityOrderNamesAndNoDirectFallback(t *testing.T) {
	links:=[]string{clashRealityFixture("VPS 本机直出"),clashRealityFixture("自定义 · 示例"),clashRealityFixture("🇯🇵 日本 01"),clashRealityFixture("DIRECT"),clashRealityFixture("VPS 本机直出")}
	blob,err:=renderClash(links);if err!=nil {t.Fatal(err)}
	var cfg struct{Proxies []map[string]any `json:"proxies"`; Groups []struct{Proxies []string `json:"proxies"`;Type string `json:"type"`} `json:"proxy-groups"`;Rules []string `json:"rules"`}
	if json.Unmarshal(blob,&cfg)!=nil || len(cfg.Proxies)!=5 || len(cfg.Groups)!=1 {t.Fatal("invalid profile")}
	if cfg.Groups[0].Type!="select" || cfg.Rules[0]!="MATCH,xnvgatebox" {t.Fatal("implicit fallback added")}
	seen:=map[string]bool{}
	for i,p:=range cfg.Proxies {
		name:=p["name"].(string);if seen[name]||name=="DIRECT" {t.Fatal("reserved or duplicate name")};seen[name]=true
		if cfg.Groups[0].Proxies[i]!=name || p["udp"]!=false || p["skip-cert-verify"]!=false || p["client-fingerprint"]!="chrome" || p["flow"]!="xtls-rprx-vision" {t.Fatal("transport or ordering changed")}
		if p["reality-opts"].(map[string]any)["short-id"]!="12345678" {t.Fatal("REALITY credentials lost")}
	}
}

func TestClashTransportsPinAndCredentialsAreNotDowngraded(t *testing.T) {
	links:=[]string{
		"vless://fbaea75a-e91c-4ad9-9410-7e43aa967766@host.example:443?type=ws&security=tls&path=%2Fws&host=cdn.example&pinSHA256="+strings.Repeat("ab",32)+"#Name%3A%20%23%5Btest%5D",
		"trojan://"+url.User("p@ss:#/?").String()+"@host.example:443?security=tls&type=grpc&serviceName=service&sni=host.example#test",
	}
	p,err:=clashProxy(links[0]);if err!=nil || p["fingerprint"]!=strings.Repeat("ab",32) || p["skip-cert-verify"]!=false {t.Fatal("certificate pin lost",err)}
	p,err=clashProxy(links[1]);if err!=nil || p["password"]!="p@ss:#/?" || p["network"]!="grpc" {t.Fatal("credential escaping or transport failed",err)}
	for _,pair:=range [][2]string{{"type=tcp","type=xhttp"},{"pbk="+base64.RawURLEncoding.EncodeToString(make([]byte,32)),"pbk=invalid"},{"flow=xtls-rprx-vision","flow=other"},{"security=reality","security=invalid"}} {
		link:=strings.Replace(clashRealityFixture("test"),pair[0],pair[1],1)
		if _,err:=renderClash([]string{link});err==nil {t.Fatal("unsupported or invalid configuration accepted")}
	}
}

func TestClashStandardVmessShareAndScopedEndpoint(t *testing.T) {
	jsonShare:=`{"v":"2","ps":"VMess","add":"host.example","port":"443","id":"fbaea75a-e91c-4ad9-9410-7e43aa967766","aid":"0","net":"ws","path":"/proxy","tls":"tls","sni":"host.example"}`
	p,err:=clashProxy("vmess://"+base64.StdEncoding.EncodeToString([]byte(jsonShare)))
	if err!=nil || p["type"]!="vmess" || p["network"]!="ws" || p["tls"]!=true {t.Fatal("standard VMess rejected",err)}
	useSettings(t);usePanel(t,&fakePanel{inbounds:[]Inbound{{ID:1,Enable:true}}})
	m:=NewManager(20,t.TempDir());tok,_:=subToken()
	w:=httptest.NewRecorder();handleSub(m)(w,httptest.NewRequest(http.MethodGet,"/sub?token="+tok+"&target=clash&bound=0",nil))
	if w.Code!=200 || !strings.Contains(w.Header().Get("Content-Type"),"yaml") || !strings.Contains(w.Body.String(),`"proxies"`) {t.Fatal("clash endpoint unavailable",w.Code)}
	w=httptest.NewRecorder();handleSub(m)(w,httptest.NewRequest(http.MethodGet,"/sub?token=wrong&target=clash",nil))
	if w.Code!=404 {t.Fatal("format bypassed authentication")}
	w=httptest.NewRecorder();handleSub(m)(w,httptest.NewRequest(http.MethodGet,"/sub?token="+tok+"&target=clash&vpn=0&direct=0&third=0",nil))
	if w.Code!=404 {t.Fatal("empty selection exported a fallback configuration")}
}

func TestClashMihomoAcceptsGeneratedProfiles(t *testing.T) {
	binary,err:=filepath.Abs(filepath.Join("..","..","tools","bin","mihomo"))
	if err!=nil {t.Fatal(err)}
	if _,err:=os.Stat(binary);err!=nil {t.Skip("install the pinned official Mihomo test client")}
	key,err:=ecdh.X25519().GenerateKey(rand.Reader);if err!=nil {t.Fatal(err)}
	link:=strings.Replace(clashRealityFixture("Reality"),base64.RawURLEncoding.EncodeToString(make([]byte,32)),base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()),1)
	for _,links:=range [][]string{{link},{"vless://fbaea75a-e91c-4ad9-9410-7e43aa967766@host.example:443?type=ws&security=tls&path=%2Fws&sni=host.example#WS"},{"trojan://public-test-password@host.example:443?type=grpc&security=tls&sni=host.example&serviceName=service#RPC"},{"vmess://fbaea75a-e91c-4ad9-9410-7e43aa967766@host.example:443?security=tls&type=tcp&encryption=auto#VMess"}} {
		dir:=t.TempDir();blob,err:=renderClash(links);if err!=nil {t.Fatal(err)}
		path:=filepath.Join(dir,"profile.yaml");if err:=os.WriteFile(path,blob,0600);err!=nil {t.Fatal(err)}
		ctx,cancel:=context.WithTimeout(context.Background(),20*time.Second)
		out,err:=exec.CommandContext(ctx,binary,"-t","-d",dir,"-f",path).CombinedOutput();cancel()
		if err!=nil {if len(out)>1000 {out=out[:1000]};t.Fatalf("pinned Mihomo rejected public fixture: %v %s",err,out)}
	}
}

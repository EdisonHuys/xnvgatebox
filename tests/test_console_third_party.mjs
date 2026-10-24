import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";

const html = fs.readFileSync(
  new URL("../vps/fanout/console.html", import.meta.url),
  "utf8",
);
const between = (a, b) =>
  html.slice(html.indexOf(a), html.indexOf(b, html.indexOf(a)));
const c = vm.createContext({
  URL,
  location: { href: "https://console.example.invalid/" },
  subscription: { url: "https://console.example.invalid/sub?token=demo-only" },
  exits: [{ slot: 1 }, { slot: 2 }],
  thirdProxies: [{ id: 10 }, { id: 11 }],
  subSelected: new Set(),
  subThirdSelected: new Set(),
  elements: {
    subAll: { checked: true },
    subDirect: { checked: true },
    subThirdAll: { checked: true },
    subFormat: { value: 'base64' },
  },
});
vm.runInContext(
  "const $ = id => elements[id];\n" +
    between("      function orderedThirdParties()", "      function thirdCheckReason(") +
    between(
      "      function selectedSubURL()",
      "      function renderSubscription()",
    ),
  c,
);
let u = new URL(c.selectedSubURL());
assert.equal(u.searchParams.get('target'),null);
c.elements.subFormat.value='clash';u=new URL(c.selectedSubURL());assert.equal(u.searchParams.get('target'),'clash');
c.elements.subFormat.value='base64';u=new URL(c.selectedSubURL());assert.equal(u.searchParams.get('target'),null);
assert.equal(u.searchParams.get("third"), "1");
assert.equal(u.searchParams.get("slots"), null);
c.elements.subAll.checked = false;
c.elements.subDirect.checked = false;
u = new URL(c.selectedSubURL());
assert.equal(
  u.searchParams.get("vpn"),
  "0",
  "third-party only scope leaked VPN Gate",
);
assert.equal(u.searchParams.get("direct"), "0");
assert.equal(u.searchParams.get("third"), "1");
c.elements.subThirdAll.checked = false;
c.subThirdSelected.add(11);
u = new URL(c.selectedSubURL());
assert.equal(u.searchParams.get("proxies"), "11");
assert.equal(u.searchParams.get("third"), null);
c.subThirdSelected.clear();
assert.equal(c.selectedSubURL(), "", "all empty still generated a scope");
c.subSelected.add(2);
u = new URL(c.selectedSubURL());
assert.equal(u.searchParams.get("slots"), "2");
assert.equal(u.searchParams.get("third"), "0");
// An inherited URL must not override newly selected groups or keep stale IDs.
c.subscription.url += "&slots=1&third=0&proxies=10&vpn=0";
c.elements.subAll.checked = true;
c.elements.subThirdAll.checked = true;
u = new URL(c.selectedSubURL());
assert.equal(u.searchParams.get("slots"), null);
assert.equal(u.searchParams.get("proxies"), null);
assert.equal(u.searchParams.get("vpn"), null);
assert.equal(u.searchParams.get("third"), "1");

// A GET begun before a new check cannot erase its accepted state.
const pending = () => {
  let resolve;
  const promise = new Promise((r) => (resolve = r));
  return { promise, resolve };
};
const old = pending(),
  fresh = pending();
let gets = 0;
const s = vm.createContext({
  thirdEpoch: 0,
  thirdProxies: [{ id: 10, check: { status: "not_tested" } }],
  thirdCountries: [],
  thirdSubmitting: new Set(),
  renderThirdParties() {},
  renderSubscription() {},
  api: (path) =>
    path.endsWith("/check")
      ? Promise.resolve({ status: "running" })
      : ++gets === 1
        ? old.promise
        : fresh.promise,
});
vm.runInContext(
  between(
    "      async function refreshThirdParties()",
    "      async function openThirdParty(",
  ),
  s,
);
const oldGet = s.refreshThirdParties(),
  post = s.checkThirdParty(10);
assert.equal(s.thirdSubmitting.has(10), true);
await new Promise(setImmediate);
fresh.resolve({ proxies: [{ id: 10, check: { status: "running" } }] });
await post;
old.resolve({ proxies: [{ id: 10, check: { status: "not_tested" } }] });
await oldGet;
assert.equal(s.thirdProxies[0].check.status, "running");
assert.equal(s.thirdSubmitting.size, 0);
console.log("Third-party subscription scope and stale-poll regressions passed");

// Group order is visible in the selection page. Returned proxy order and
// changing check results must not move a custom entry between positions.
const scope = between('<h2>选择订阅范围</h2>', '<div class="card pad qr">');
assert(scope.indexOf('id="subDirect"') < scope.indexOf('id="subThirdAll"'));
assert(scope.indexOf('id="subThirdRows"') < scope.indexOf('id="subAll"'));
const elements = Object.fromEntries([
  "managedDirectSection", "managedThirdSection", "managedDirectExit", "managedThirdExits", "thirdRows",
].map(id => [id, {innerHTML: "", hidden: false}]));
const ui = vm.createContext({
  elements, summary: {backend: "native"}, resourcePublicIP: "192.0.2.100",
  hostDirect: [{id: 99, enable: true, port: 30999}],
  thirdProxies: [
    {id: 11, name: 'second <custom>', enabled: false, entry_port: 30111, check: {status: "passed", actual_ip: "198.51.100.11", ip_type: "unknown"}},
    {id: 10, name: 'nov proxy-los', enabled: true, entry_port: 30110, subscribe: true, check: {status: "not_tested"}},
  ],
  thirdSubmitting: new Set(), types: {unknown: "未知 / unknown"}, labels: {},
  esc: s => String(s ?? "").replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll('"', "&quot;"),
  country: code => code, date: () => "demo-time", badge: status => `[${status}]`, checkButtonLabel: r => r.status,
});
vm.runInContext("const $ = id => elements[id];\n" +
  between("      function orderedThirdParties()", "      function render()") +
  between("      function renderThirdParties()", "      async function refreshThirdParties()"), ui);
ui.renderThirdParties();
const cards = () => elements.managedThirdExits.innerHTML;
assert(cards().indexOf("nov proxy-los") < cards().indexOf("second &lt;custom>"));
assert(cards().includes("自定义 · SOCKS5"));
assert(cards().includes('data-third-edit="10"'));
assert(cards().includes('data-third-toggle="11"'));
assert(cards().includes("未知 / unknown"));
const second = cards().slice(cards().indexOf("second &lt;custom>"));
assert(second.includes("已停用"), "old passed observation hid the disabled state");
ui.thirdProxies[1].name = "updated from resource editor";
ui.thirdSubmitting.add(10);
ui.renderThirdParties();
assert(cards().includes("updated from resource editor"), "resource refresh did not reach exit management");
assert(cards().includes('disabled aria-busy="true"'), "exit card lost immediate check feedback");
ui.thirdProxies = [];
ui.renderThirdParties();
assert(!cards().includes('data-third-edit="10"'), "deleted resource stayed in exit management");
assert(elements.managedDirectExit.innerHTML.includes("192.0.2.100"));
console.log("Unified exit sources, shared state and stable source order passed");

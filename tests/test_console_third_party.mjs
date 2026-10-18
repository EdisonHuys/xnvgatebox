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
  },
});
vm.runInContext(
  "const $ = id => elements[id];\n" +
    between(
      "      function selectedSubURL()",
      "      function renderSubscription()",
    ),
  c,
);
let u = new URL(c.selectedSubURL());
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

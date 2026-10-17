# fanout upstream notice

This directory is based on `byJoey/fanout` commit `d7ce5224caf3469b19876982abdf8d1be16c4a8b`.

The upstream project is MIT licensed. Its original `LICENSE` is retained in this directory.
The xnvgatebox integration and subsequent VPS changes include:

- load candidates from the shared validated node pool instead of fetching a second VPN Gate pool;
- keep Exit Slot ports on an explicit fixed mapping and bind SOCKS listeners to loopback;
- perform two-endpoint HTTPS expected/actual exit checks;
- add a host and namespace kill-switch so a dead OpenVPN cannot use the host NAT as a fallback.

- default-block native Xray routing with stable slot outbound tags;
- authenticated standard Xray JSON export and a project-owned standalone package/installer;
- local shared-pool refresh, without runtime fallback to an independent scraper;
- disable upstream single-binary updates in favor of whole-project package updates.

The upstream source remains identifiable so license obligations and future diff review are clear.

The redesigned xnvgatebox console and quantity/diagnostic controller are project changes. Local QR generation uses github.com/skip2/go-qrcode (MIT); its license is retained as vps/licenses/LICENSE.go-qrcode.

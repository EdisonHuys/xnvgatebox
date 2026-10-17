package main

import "testing"

func TestLegacyDefaultNamesKeepExplicitUserChoice(t *testing.T) {
	for _, tc := range []struct{ mode, remark, want string }{
		{"", "slot-01", "auto"}, {"", "slot-03", "auto"}, {"", "slot-32", "auto"},
		{"custom", "slot-01", "custom"}, {"custom", "🇯🇵 我的线路", "custom"},
		{"", "slot-00", "custom"}, {"", "slot-33", "custom"}, {"", "slot-1", "custom"},
		{"", "slot-01-office", "custom"}, {"", "我的出口", "custom"},
		{"auto", "我的旧名", "auto"},
	} {
		if got := inboundNameMode(tc.mode, tc.remark); got != tc.want {
			t.Errorf("mode=%q remark=%q: got %s, want %s", tc.mode, tc.remark, got, tc.want)
		}
	}
}

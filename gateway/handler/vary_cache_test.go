package handler

import "testing"

func TestVaryBlocksCache(t *testing.T) {
	cases := []struct {
		name  string
		vary  string
		block bool
	}{
		{"empty", "", false},
		{"accept-encoding only", "Accept-Encoding", false},
		{"accept-encoding case-insensitive", "accept-encoding", false},
		{"accept-encoding with spaces", " Accept-Encoding ", false},
		{"accept blocks", "Accept", true},
		{"accept-language blocks", "Accept-Language", true},
		{"origin blocks", "Origin", true},
		{"accept-encoding plus accept blocks", "Accept-Encoding, Accept", true},
		{"multiple all accept-encoding-equivalent does not block", "Accept-Encoding, accept-encoding", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := varyBlocksCache(tc.vary)
			if got != tc.block {
				t.Errorf("varyBlocksCache(%q) = %v, want %v", tc.vary, got, tc.block)
			}
		})
	}
}

package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// --- isRootOrAdmin ---

func TestIsRootOrAdmin(t *testing.T) {
	cases := []struct {
		role string
		want bool
	}{
		{"root", true},
		{"admin", true},
		{"user", false},
		{"", false},
		{"ROOT", false},
		{"ADMIN", false},
	}

	for _, tc := range cases {
		got := isRootOrAdmin(tc.role)
		assert.Equal(t, tc.want, got, "role=%q", tc.role)
	}
}

package handler

import (
	"testing"

	"github.com/oryca/oryca/control-plane/model"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// --- canSetUserRole ---

func TestCanSetUserRole(t *testing.T) {
	cases := []struct {
		actorRole  string
		targetRole string
		want       bool
	}{
		// root ได้ทุก role
		{"root", "root", true},
		{"root", "admin", true},
		{"root", "user", true},
		// admin ได้เฉพาะ user
		{"admin", "user", true},
		{"admin", "admin", false},
		{"admin", "root", false},
		// user ไม่ได้เลย
		{"user", "user", false},
		{"", "user", false},
	}

	for _, tc := range cases {
		got := canSetUserRole(tc.actorRole, tc.targetRole)
		assert.Equal(t, tc.want, got, "actorRole=%q targetRole=%q", tc.actorRole, tc.targetRole)
	}
}

// --- canDeleteUser ---

func TestCanDeleteUser(t *testing.T) {
	idA := bson.NewObjectID()
	idB := bson.NewObjectID()

	cases := []struct {
		name   string
		actor  *model.User
		target *model.User
		want   bool
	}{
		{
			name:   "ลบตัวเองไม่ได้",
			actor:  &model.User{ID: idA, Role: "root"},
			target: &model.User{ID: idA, Role: "user"},
			want:   false,
		},
		{
			name:   "ลบ root ไม่ได้ แม้ actor เป็น root",
			actor:  &model.User{ID: idA, Role: "root"},
			target: &model.User{ID: idB, Role: "root"},
			want:   false,
		},
		{
			name:   "root ลบ admin ได้",
			actor:  &model.User{ID: idA, Role: "root"},
			target: &model.User{ID: idB, Role: "admin"},
			want:   true,
		},
		{
			name:   "root ลบ user ได้",
			actor:  &model.User{ID: idA, Role: "root"},
			target: &model.User{ID: idB, Role: "user"},
			want:   true,
		},
		{
			name:   "admin ลบ user ได้",
			actor:  &model.User{ID: idA, Role: "admin"},
			target: &model.User{ID: idB, Role: "user"},
			want:   true,
		},
		{
			name:   "admin ลบ admin อื่นไม่ได้",
			actor:  &model.User{ID: idA, Role: "admin"},
			target: &model.User{ID: idB, Role: "admin"},
			want:   false,
		},
		{
			name:   "user ลบ user อื่นไม่ได้",
			actor:  &model.User{ID: idA, Role: "user"},
			target: &model.User{ID: idB, Role: "user"},
			want:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, canDeleteUser(tc.actor, tc.target))
		})
	}
}

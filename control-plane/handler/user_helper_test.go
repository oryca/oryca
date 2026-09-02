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
		// admin creates users only
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

// --- canSetNewRole ---

func TestCanSetNewRole(t *testing.T) {
	cases := []struct {
		name       string
		actorRole  string
		targetRole string
		newRole    string
		want       bool
	}{
		{"admin cannot promote a user to admin", "admin", "user", "admin", false},
		{"admin cannot keep another admin on admin", "admin", "admin", "admin", false},
		{"admin keeps a user on user", "admin", "user", "user", true},
		{"admin cannot hand out root", "admin", "admin", "root", false},
		{"root promotes a user to admin", "root", "user", "admin", true},
		{"a user may set nothing", "user", "user", "user", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actor := &model.User{ID: bson.NewObjectID(), Role: tc.actorRole}
			target := &model.User{ID: bson.NewObjectID(), Role: tc.targetRole}
			assert.Equal(t, tc.want, canSetNewRole(actor, target, tc.newRole))
		})
	}

	// its own row is the one exception: an admin keeps the role it already carries, or steps down
	self := &model.User{ID: bson.NewObjectID(), Role: "admin"}
	assert.True(t, canSetNewRole(self, self, "admin"))
	assert.True(t, canSetNewRole(self, self, "user"))
	assert.False(t, canSetNewRole(self, self, "root"))
}

// --- canUpdateTarget ---

func TestCanUpdateTarget(t *testing.T) {
	idA := bson.NewObjectID()
	idB := bson.NewObjectID()

	cases := []struct {
		name    string
		ctxUser *model.User
		target  *model.User
		want    bool
	}{
		{
			name:    "admin cannot update a root",
			ctxUser: &model.User{ID: idA, Role: "admin"},
			target:  &model.User{ID: idB, Role: "root"},
			want:    false,
		},
		{
			name:    "admin cannot update another admin",
			ctxUser: &model.User{ID: idA, Role: "admin"},
			target:  &model.User{ID: idB, Role: "admin"},
			want:    false,
		},
		{
			name:    "admin updates itself",
			ctxUser: &model.User{ID: idA, Role: "admin"},
			target:  &model.User{ID: idA, Role: "admin"},
			want:    true,
		},
		{
			name:    "admin updates a user",
			ctxUser: &model.User{ID: idA, Role: "admin"},
			target:  &model.User{ID: idB, Role: "user"},
			want:    true,
		},
		{
			name:    "root updates a root",
			ctxUser: &model.User{ID: idA, Role: "root"},
			target:  &model.User{ID: idB, Role: "root"},
			want:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, canUpdateTarget(tc.ctxUser, tc.target))
		})
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
			name:   "admin cannot delete another admin",
			actor:  &model.User{ID: idA, Role: "admin"},
			target: &model.User{ID: idB, Role: "admin"},
			want:   false,
		},
		{
			name:   "admin cannot delete a root",
			actor:  &model.User{ID: idA, Role: "admin"},
			target: &model.User{ID: idB, Role: "root"},
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

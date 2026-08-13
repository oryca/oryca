package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestToUserFreshnessCache(t *testing.T) {
	t.Run("nil PackageID becomes empty string", func(t *testing.T) {
		u := &User{ID: bson.NewObjectID()}
		got := ToUserFreshnessCache(u)
		assert.Equal(t, u.ID.Hex(), got.ID)
		assert.Equal(t, "", got.PackageID)
		assert.Nil(t, got.Verified)
		assert.Nil(t, got.Enabled)
		assert.Nil(t, got.ExpiredAt)
	})

	t.Run("fields pass through unchanged", func(t *testing.T) {
		pid := bson.NewObjectID()
		verified := true
		enabled := false
		expiredAt := time.Now().UTC()
		u := &User{
			ID:        bson.NewObjectID(),
			PackageID: &pid,
			Verified:  &verified,
			Enabled:   &enabled,
			ExpiredAt: &expiredAt,
		}
		got := ToUserFreshnessCache(u)
		assert.Equal(t, pid.Hex(), got.PackageID)
		assert.Equal(t, &verified, got.Verified)
		assert.Equal(t, &enabled, got.Enabled)
		assert.Equal(t, &expiredAt, got.ExpiredAt)
	})
}

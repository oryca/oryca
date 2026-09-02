package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oryca/oryca/control-plane/model"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ---- mocks ----

type mockCronLock struct {
	acquireOK    bool
	acquireErr   error
	releaseCalls int
}

func (m *mockCronLock) Acquire(_ context.Context, _ string, _ time.Duration) (string, bool, error) {
	return "tok", m.acquireOK, m.acquireErr
}
func (m *mockCronLock) Release(_ context.Context, _, _ string) error {
	m.releaseCalls++
	return nil
}

type mockCronNotifier struct {
	calls []*model.NotificationCreate
}

func (m *mockCronNotifier) Create(_ context.Context, in *model.NotificationCreate) error {
	m.calls = append(m.calls, in)
	return nil
}

type mockCronApiKeyRepo struct {
	pages       [][]*model.ApiKey // index = offset / limit
	lastOffsets []int
}

func (m *mockCronApiKeyRepo) FindExpiring(_ context.Context, _, _ time.Time, limit, offset int) ([]*model.ApiKey, error) {
	m.lastOffsets = append(m.lastOffsets, offset)
	idx := offset / limit
	if idx >= len(m.pages) {
		return nil, nil
	}
	return m.pages[idx], nil
}

func newExpiringKey(name string, owner *bson.ObjectID, exp *time.Time) *model.ApiKey {
	return &model.ApiKey{ID: bson.NewObjectID(), Name: name, OwnerBy: owner, ExpiredAt: exp}
}

// ---- tests ----

func TestRunApiKeyExpiring_LockNotAcquired(t *testing.T) {
	lock := &mockCronLock{acquireOK: false}
	notifier := &mockCronNotifier{}
	repo := &mockCronApiKeyRepo{}
	nc := NewNotificationCron(repo, lock, notifier)

	nc.runApiKeyExpiring()

	if len(notifier.calls) != 0 {
		t.Fatalf("expected no notifications when lock not acquired, got %d", len(notifier.calls))
	}
	if lock.releaseCalls != 0 {
		t.Errorf("must NOT release a lock we did not acquire, got %d releases", lock.releaseCalls)
	}
	if len(repo.lastOffsets) != 0 {
		t.Errorf("must not query when lock not acquired")
	}
}

func TestRunApiKeyExpiring_NotifiesAndReleases(t *testing.T) {
	owner := bson.NewObjectID()
	exp := time.Now().UTC().Add(3 * 24 * time.Hour)
	lock := &mockCronLock{acquireOK: true}
	notifier := &mockCronNotifier{}
	repo := &mockCronApiKeyRepo{pages: [][]*model.ApiKey{{
		newExpiringKey("prod-key", &owner, &exp),
		newExpiringKey("no-owner", nil, &exp),    // ต้องข้าม (ไม่มีเจ้าของ)
		newExpiringKey("no-expiry", &owner, nil), // ต้องข้าม (ไม่มี expiredAt)
	}}}
	nc := NewNotificationCron(repo, lock, notifier)

	nc.runApiKeyExpiring()

	if len(notifier.calls) != 1 {
		t.Fatalf("expected exactly 1 notification (skip nil owner/expiry), got %d", len(notifier.calls))
	}
	got := notifier.calls[0]
	if got.Topic != model.NotificationTopicApiKeyExpiring {
		t.Errorf("topic = %q, want apikey_expiring", got.Topic)
	}
	if got.UserID != owner {
		t.Errorf("must notify the key owner")
	}
	wantDedup := "apikey_expiring:" + got.Detail["apiKeyId"].(string) + ":" + exp.UTC().Format("2006-01-02")
	if got.DedupKey != wantDedup {
		t.Errorf("dedupKey = %q, want %q", got.DedupKey, wantDedup)
	}
	if got.CreatedBy != model.SystemObjectID {
		t.Errorf("createdBy must be SystemObjectID")
	}
	if lock.releaseCalls != 1 {
		t.Errorf("lock must be released exactly once, got %d", lock.releaseCalls)
	}
}

func TestRunApiKeyExpiring_Batching(t *testing.T) {
	owner := bson.NewObjectID()
	exp := time.Now().UTC().Add(24 * time.Hour)

	// หน้าแรกเต็ม batch → ต้องดึงหน้าถัดไป; หน้าสองไม่เต็ม → หยุด
	full := make([]*model.ApiKey, cronExpiringBatchSize)
	for i := range full {
		full[i] = newExpiringKey("k", &owner, &exp)
	}
	partial := []*model.ApiKey{newExpiringKey("last", &owner, &exp)}

	lock := &mockCronLock{acquireOK: true}
	notifier := &mockCronNotifier{}
	repo := &mockCronApiKeyRepo{pages: [][]*model.ApiKey{full, partial}}
	nc := NewNotificationCron(repo, lock, notifier)

	nc.runApiKeyExpiring()

	wantNotifs := cronExpiringBatchSize + 1
	if len(notifier.calls) != wantNotifs {
		t.Fatalf("expected %d notifications across 2 pages, got %d", wantNotifs, len(notifier.calls))
	}
	// ต้อง query offset 0 แล้ว batchSize (ยืนยันเดินหน้า pagination ถูก)
	if len(repo.lastOffsets) != 2 || repo.lastOffsets[0] != 0 || repo.lastOffsets[1] != cronExpiringBatchSize {
		t.Errorf("offsets = %v, want [0 %d]", repo.lastOffsets, cronExpiringBatchSize)
	}
}

func TestRunApiKeyExpiring_LockErrorNoQuery(t *testing.T) {
	lock := &mockCronLock{acquireErr: errors.New("redis down")}
	notifier := &mockCronNotifier{}
	repo := &mockCronApiKeyRepo{}
	nc := NewNotificationCron(repo, lock, notifier)

	nc.runApiKeyExpiring()

	if len(repo.lastOffsets) != 0 || len(notifier.calls) != 0 {
		t.Errorf("must not query/notify when lock acquire errors")
	}
	if lock.releaseCalls != 0 {
		t.Errorf("must not release on acquire error")
	}
}

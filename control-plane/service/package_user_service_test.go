package service

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/oryca/oryca/control-plane/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// --- mocks (mockPackageFinder is reused from package_svc_link_service_test.go) ---

type mockPackageUserCountRepo struct{ mock.Mock }

func (m *mockPackageUserCountRepo) IncrUserCount(ctx context.Context, id bson.ObjectID, delta int64) error {
	return m.Called(ctx, id, delta).Error(0)
}

type mockPackageUserGatewayPublisher struct{ mock.Mock }

func (m *mockPackageUserGatewayPublisher) Publish(eventType string, payload any) {
	m.Called(eventType, payload)
}

// fakePackageUserRepo implements packageUserRepo — only BulkSetPackageID/
// BulkUnsetPackageID/CountByOldPackageIDs are exercised here, FindByPackageID isn't
// used by AssignUsers/RemoveUsers.
type fakePackageUserRepo struct {
	mock.Mock
}

func (m *fakePackageUserRepo) FindByPackageID(ctx context.Context, packageID bson.ObjectID, params url.Values) ([]*model.User, int64, error) {
	args := m.Called(ctx, packageID, params)
	if v := args.Get(0); v != nil {
		return v.([]*model.User), args.Get(1).(int64), args.Error(2)
	}
	return nil, 0, args.Error(2)
}

func (m *fakePackageUserRepo) BulkSetPackageID(ctx context.Context, userIDs []bson.ObjectID, packageID bson.ObjectID, now time.Time, updatedBy bson.ObjectID) (int64, error) {
	args := m.Called(ctx, userIDs, packageID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *fakePackageUserRepo) BulkUnsetPackageID(ctx context.Context, packageID bson.ObjectID, userIDs []bson.ObjectID, now time.Time, updatedBy bson.ObjectID) (int64, error) {
	args := m.Called(ctx, packageID, userIDs)
	return args.Get(0).(int64), args.Error(1)
}

func (m *fakePackageUserRepo) CountByOldPackageIDs(ctx context.Context, userIDs []bson.ObjectID, newPackageID bson.ObjectID) (map[bson.ObjectID]int64, error) {
	args := m.Called(ctx, userIDs, newPackageID)
	if v := args.Get(0); v != nil {
		return v.(map[bson.ObjectID]int64), args.Error(1)
	}
	return nil, args.Error(1)
}

func newPackageUserService(repo *fakePackageUserRepo, pkgRepo *mockPackageFinder, pkgCountRepo *mockPackageUserCountRepo, pub *mockPackageUserGatewayPublisher) *PackageUserService {
	return NewPackageUserService(repo, pkgRepo, pkgCountRepo, nil, nil, nil, pub)
}

// --- AssignUsers / RemoveUsers publish exactly one event ---

func TestPackageUserServiceAssignUsersPublishesOneInvalidateEvent(t *testing.T) {
	// Regression test: AssignUsers is a bulk operation — a groupID can expand to
	// hundreds of users in one BulkSetPackageID call. Publishing must fire exactly
	// once per call, carrying the full deduped id set, never once per user (that's
	// the event-storm bug already fixed for reload_services).
	ctx := context.Background()
	pid := bson.NewObjectID()
	id1, id2 := bson.NewObjectID(), bson.NewObjectID()
	ctxUser := &model.User{ID: bson.NewObjectID()}

	repo := new(fakePackageUserRepo)
	pkgRepo := new(mockPackageFinder)
	pkgCountRepo := new(mockPackageUserCountRepo)
	pub := new(mockPackageUserGatewayPublisher)

	pkgRepo.On("FindByID", ctx, pid).Return(&model.Package{ID: pid, Name: "Standard"}, nil)
	repo.On("CountByOldPackageIDs", ctx, mock.Anything, pid).Return(map[bson.ObjectID]int64{}, nil)
	repo.On("BulkSetPackageID", ctx, mock.Anything, pid).Return(int64(2), nil)
	pkgCountRepo.On("IncrUserCount", ctx, pid, int64(2)).Return(nil)

	hexIDs := []string{id1.Hex(), id2.Hex()}
	pub.On("Publish", SyncEventTypeUserInvalidate, mock.MatchedBy(func(p *model.UserIDsPayload) bool {
		if len(p.UserIDs) != len(hexIDs) {
			return false
		}
		seen := map[string]bool{}
		for _, id := range p.UserIDs {
			seen[id] = true
		}
		for _, id := range hexIDs {
			if !seen[id] {
				return false
			}
		}
		return true
	})).Return()

	svc := newPackageUserService(repo, pkgRepo, pkgCountRepo, pub)
	assigned, skipped, err := svc.AssignUsers(ctx, pid.Hex(), []string{id1.Hex(), id2.Hex()}, nil, ctxUser)

	require.NoError(t, err)
	assert.Equal(t, int64(2), assigned)
	assert.Equal(t, 0, skipped)
	pub.AssertNumberOfCalls(t, "Publish", 1)
}

func TestPackageUserServiceRemoveUsersPublishesOneInvalidateEvent(t *testing.T) {
	ctx := context.Background()
	pid := bson.NewObjectID()
	id1, id2, id3 := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	ctxUser := &model.User{ID: bson.NewObjectID()}

	repo := new(fakePackageUserRepo)
	pkgRepo := new(mockPackageFinder)
	pkgCountRepo := new(mockPackageUserCountRepo)
	pub := new(mockPackageUserGatewayPublisher)

	pkgRepo.On("FindByID", ctx, pid).Return(&model.Package{ID: pid, Name: "Standard"}, nil)
	repo.On("BulkUnsetPackageID", ctx, pid, mock.Anything).Return(int64(3), nil)
	pkgCountRepo.On("IncrUserCount", ctx, pid, int64(-3)).Return(nil)
	pub.On("Publish", SyncEventTypeUserInvalidate, mock.Anything).Return()

	svc := newPackageUserService(repo, pkgRepo, pkgCountRepo, pub)
	removed, skipped, err := svc.RemoveUsers(ctx, pid.Hex(), []string{id1.Hex(), id2.Hex(), id3.Hex()}, nil, ctxUser)

	require.NoError(t, err)
	assert.Equal(t, int64(3), removed)
	assert.Equal(t, 0, skipped)
	pub.AssertNumberOfCalls(t, "Publish", 1)
}

func TestPackageUserServiceNoPublishWhenNothingChanged(t *testing.T) {
	ctx := context.Background()
	pid := bson.NewObjectID()
	ctxUser := &model.User{ID: bson.NewObjectID()}

	repo := new(fakePackageUserRepo)
	pkgRepo := new(mockPackageFinder)
	pkgCountRepo := new(mockPackageUserCountRepo)
	pub := new(mockPackageUserGatewayPublisher)

	pkgRepo.On("FindByID", ctx, pid).Return(&model.Package{ID: pid}, nil)
	repo.On("CountByOldPackageIDs", ctx, mock.Anything, pid).Return(map[bson.ObjectID]int64{}, nil)
	repo.On("BulkSetPackageID", ctx, mock.Anything, pid).Return(int64(0), nil)

	svc := newPackageUserService(repo, pkgRepo, pkgCountRepo, pub)
	assigned, _, err := svc.AssignUsers(ctx, pid.Hex(), []string{bson.NewObjectID().Hex()}, nil, ctxUser)

	require.NoError(t, err)
	assert.Equal(t, int64(0), assigned)
	pub.AssertNotCalled(t, "Publish", mock.Anything, mock.Anything)
}

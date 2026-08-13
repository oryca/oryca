package service

import (
	"context"
	"net/url"
	"testing"

	"github.com/oryca/oryca/control-plane/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func testUser() *model.User {
	return &model.User{ID: bson.NewObjectID()}
}

// --- mocks ---

type mockPackageSvcLinkRepo struct{ mock.Mock }

func (m *mockPackageSvcLinkRepo) Aggregate(ctx context.Context, packageID *bson.ObjectID, params url.Values) ([]*model.PackageSvcLinkDetail, int64, error) {
	args := m.Called(ctx, packageID, params)
	if v := args.Get(0); v != nil {
		return v.([]*model.PackageSvcLinkDetail), args.Get(1).(int64), args.Error(2)
	}
	return nil, 0, args.Error(2)
}
func (m *mockPackageSvcLinkRepo) FindByPackageAndService(ctx context.Context, packageID, serviceID bson.ObjectID) (*model.PackageSvcLink, error) {
	args := m.Called(ctx, packageID, serviceID)
	if v := args.Get(0); v != nil {
		return v.(*model.PackageSvcLink), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockPackageSvcLinkRepo) FindPackageIDsByServiceID(ctx context.Context, serviceID bson.ObjectID) ([]string, error) {
	args := m.Called(ctx, serviceID)
	if v := args.Get(0); v != nil {
		return v.([]string), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockPackageSvcLinkRepo) Insert(ctx context.Context, doc *model.PackageSvcLink) error {
	return m.Called(ctx, doc).Error(0)
}
func (m *mockPackageSvcLinkRepo) Update(ctx context.Context, id bson.ObjectID, fields bson.M) error {
	return m.Called(ctx, id, fields).Error(0)
}
func (m *mockPackageSvcLinkRepo) Delete(ctx context.Context, packageID, serviceID bson.ObjectID) error {
	return m.Called(ctx, packageID, serviceID).Error(0)
}

type mockPackageGatewayServiceFinder struct{ mock.Mock }

func (m *mockPackageGatewayServiceFinder) FindByID(ctx context.Context, id bson.ObjectID) (*model.GatewayService, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*model.GatewayService), args.Error(1)
	}
	return nil, args.Error(1)
}

type mockPackageFinder struct{ mock.Mock }

func (m *mockPackageFinder) FindByID(ctx context.Context, id bson.ObjectID) (*model.Package, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*model.Package), args.Error(1)
	}
	return nil, args.Error(1)
}

type mockPackageIDsSyncer struct{ mock.Mock }

func (m *mockPackageIDsSyncer) SyncServicePackageIDs(ctx context.Context, svc *model.GatewayService, packageIDs []string) error {
	return m.Called(ctx, svc, packageIDs).Error(0)
}

type mockPackageSvcCountRepo struct{ mock.Mock }

func (m *mockPackageSvcCountRepo) IncrServiceCount(ctx context.Context, id bson.ObjectID, delta int64) error {
	return m.Called(ctx, id, delta).Error(0)
}

// --- helpers ---

func newTestPackageSvcLinkService() (*PackageSvcLinkService, *mockPackageSvcLinkRepo, *mockPackageGatewayServiceFinder, *mockPackageFinder, *mockPackageIDsSyncer, *mockPackageSvcCountRepo) {
	repo := &mockPackageSvcLinkRepo{}
	serviceSvc := &mockPackageGatewayServiceFinder{}
	packageRepo := &mockPackageFinder{}
	pkgRepo := &mockPackageSvcCountRepo{}
	redisSync := &mockPackageIDsSyncer{}
	svc := NewPackageSvcLinkService(repo, serviceSvc, packageRepo, pkgRepo, redisSync, nil, nil)
	return svc, repo, serviceSvc, packageRepo, redisSync, pkgRepo
}

// --- Create ---

func TestPackageSvcLinkService_Create_SyncsWithFullServiceObject(t *testing.T) {
	svc, repo, serviceSvc, packageRepo, redisSync, pkgRepo := newTestPackageSvcLinkService()
	ctx := context.Background()
	pid, sid := bson.NewObjectID(), bson.NewObjectID()
	gatewaySvc := testGatewayService(sid)
	pkgIDs := []string{"pkg-a"}

	repo.On("FindByPackageAndService", ctx, pid, sid).Return(nil, assert.AnError)
	serviceSvc.On("FindByID", ctx, sid).Return(gatewaySvc, nil)
	repo.On("Insert", ctx, mock.AnythingOfType("*model.PackageSvcLink")).Return(nil)
	pkgRepo.On("IncrServiceCount", ctx, pid, int64(1)).Return(nil)
	repo.On("FindPackageIDsByServiceID", ctx, sid).Return(pkgIDs, nil)
	// SyncServicePackageIDs must receive the *same* full service object fetched above,
	// not just its ID — this is what makes the write independent of any prior Redis state.
	redisSync.On("SyncServicePackageIDs", ctx, gatewaySvc, pkgIDs).Return(nil)
	packageRepo.On("FindByID", ctx, pid).Return(nil, assert.AnError)

	body := &model.PackageSvcLinkCreate{ServiceID: sid.Hex(), Paths: []*model.PackagePath{{Path: "/foo", Methods: []string{"GET"}}}}
	_, err := svc.Create(ctx, pid.Hex(), body, testUser())

	require.NoError(t, err)
	redisSync.AssertCalled(t, "SyncServicePackageIDs", ctx, gatewaySvc, pkgIDs)
}

func TestPackageSvcLinkService_Create_DuplicateLinkSkipsSync(t *testing.T) {
	svc, repo, _, _, redisSync, _ := newTestPackageSvcLinkService()
	ctx := context.Background()
	pid, sid := bson.NewObjectID(), bson.NewObjectID()

	repo.On("FindByPackageAndService", ctx, pid, sid).Return(&model.PackageSvcLink{ID: bson.NewObjectID()}, nil)

	body := &model.PackageSvcLinkCreate{ServiceID: sid.Hex()}
	_, err := svc.Create(ctx, pid.Hex(), body, testUser())

	assert.ErrorIs(t, err, ErrPackageSvcLinkDuplicate)
	redisSync.AssertNotCalled(t, "SyncServicePackageIDs", mock.Anything, mock.Anything, mock.Anything)
}

// --- Delete ---

func TestPackageSvcLinkService_Delete_SyncsWithFullServiceObject(t *testing.T) {
	svc, repo, serviceSvc, packageRepo, redisSync, pkgRepo := newTestPackageSvcLinkService()
	ctx := context.Background()
	pid, sid := bson.NewObjectID(), bson.NewObjectID()
	gatewaySvc := testGatewayService(sid)
	pkgIDs := []string{"pkg-a"}

	repo.On("FindByPackageAndService", ctx, pid, sid).Return(&model.PackageSvcLink{ID: bson.NewObjectID()}, nil)
	serviceSvc.On("FindByID", ctx, sid).Return(gatewaySvc, nil)
	repo.On("Delete", ctx, pid, sid).Return(nil)
	pkgRepo.On("IncrServiceCount", ctx, pid, int64(-1)).Return(nil)
	repo.On("FindPackageIDsByServiceID", ctx, sid).Return(pkgIDs, nil)
	redisSync.On("SyncServicePackageIDs", ctx, gatewaySvc, pkgIDs).Return(nil)
	packageRepo.On("FindByID", ctx, pid).Return(nil, assert.AnError)

	err := svc.Delete(ctx, pid.Hex(), sid.Hex(), testUser())

	require.NoError(t, err)
	redisSync.AssertCalled(t, "SyncServicePackageIDs", ctx, gatewaySvc, pkgIDs)
}

func TestPackageSvcLinkService_Delete_SkipsSyncWhenServiceNotFound(t *testing.T) {
	// Regression test: when the gateway service can no longer be fetched (e.g. it was already
	// deleted — GatewayServiceService.Delete already removes its Redis entry via DeleteService),
	// syncServicePackages must not run — there is no *model.GatewayService to rebuild the
	// payload from, and the entry shouldn't exist in Redis anymore anyway.
	svc, repo, serviceSvc, _, redisSync, pkgRepo := newTestPackageSvcLinkService()
	ctx := context.Background()
	pid, sid := bson.NewObjectID(), bson.NewObjectID()

	repo.On("FindByPackageAndService", ctx, pid, sid).Return(&model.PackageSvcLink{ID: bson.NewObjectID()}, nil)
	serviceSvc.On("FindByID", ctx, sid).Return(nil, assert.AnError)
	repo.On("Delete", ctx, pid, sid).Return(nil)
	pkgRepo.On("IncrServiceCount", ctx, pid, int64(-1)).Return(nil)

	err := svc.Delete(ctx, pid.Hex(), sid.Hex(), testUser())

	require.NoError(t, err)
	redisSync.AssertNotCalled(t, "SyncServicePackageIDs", mock.Anything, mock.Anything, mock.Anything)
	repo.AssertNotCalled(t, "FindPackageIDsByServiceID", mock.Anything, mock.Anything)
}

// --- syncServicePackages error paths ---

func TestPackageSvcLinkService_SyncServicePackages_SkipsRedisWriteWhenPackageLookupFails(t *testing.T) {
	svc, repo, _, _, redisSync, _ := newTestPackageSvcLinkService()
	ctx := context.Background()
	gatewaySvc := testGatewayService(bson.NewObjectID())

	repo.On("FindPackageIDsByServiceID", ctx, gatewaySvc.ID).Return(nil, assert.AnError)

	svc.syncServicePackages(ctx, gatewaySvc)

	redisSync.AssertNotCalled(t, "SyncServicePackageIDs", mock.Anything, mock.Anything, mock.Anything)
}

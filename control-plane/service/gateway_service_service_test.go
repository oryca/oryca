package service

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/oryca/oryca/control-plane/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// --- mocks ---

type mockGatewayServiceRepo struct{ mock.Mock }

func (m *mockGatewayServiceRepo) FindAll(ctx context.Context, params url.Values) ([]*model.GatewayService, int64, error) {
	args := m.Called(ctx, params)
	if v := args.Get(0); v != nil {
		return v.([]*model.GatewayService), args.Get(1).(int64), args.Error(2)
	}
	return nil, 0, args.Error(2)
}
func (m *mockGatewayServiceRepo) FindAllActive(ctx context.Context) ([]*model.GatewayService, error) {
	args := m.Called(ctx)
	if v := args.Get(0); v != nil {
		return v.([]*model.GatewayService), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockGatewayServiceRepo) FindAllWithSource(ctx context.Context, params url.Values) ([]*model.GatewayServiceWithSource, int64, error) {
	args := m.Called(ctx, params)
	if v := args.Get(0); v != nil {
		return v.([]*model.GatewayServiceWithSource), args.Get(1).(int64), args.Error(2)
	}
	return nil, 0, args.Error(2)
}
func (m *mockGatewayServiceRepo) FindByID(ctx context.Context, id bson.ObjectID) (*model.GatewayService, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*model.GatewayService), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockGatewayServiceRepo) FindByIDWithSource(ctx context.Context, id bson.ObjectID) (*model.GatewayServiceWithSource, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*model.GatewayServiceWithSource), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockGatewayServiceRepo) FindByBasePath(ctx context.Context, basePath string) (*model.GatewayService, error) {
	args := m.Called(ctx, basePath)
	if v := args.Get(0); v != nil {
		return v.(*model.GatewayService), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockGatewayServiceRepo) CountBySourceAlias(ctx context.Context, sourceAlias string) (int64, error) {
	args := m.Called(ctx, sourceAlias)
	return args.Get(0).(int64), args.Error(1)
}
func (m *mockGatewayServiceRepo) Insert(ctx context.Context, doc *model.GatewayService) error {
	return m.Called(ctx, doc).Error(0)
}
func (m *mockGatewayServiceRepo) Update(ctx context.Context, id bson.ObjectID, fields bson.M) error {
	return m.Called(ctx, id, fields).Error(0)
}
func (m *mockGatewayServiceRepo) SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	return m.Called(ctx, id, deletedAt, deletedBy).Error(0)
}
func (m *mockGatewayServiceRepo) HardDelete(ctx context.Context, id bson.ObjectID) error {
	return m.Called(ctx, id).Error(0)
}

type mockServiceSourceRepo struct{ mock.Mock }

func (m *mockServiceSourceRepo) FindByAlias(ctx context.Context, alias string) (*model.GatewaySource, error) {
	args := m.Called(ctx, alias)
	if v := args.Get(0); v != nil {
		return v.(*model.GatewaySource), args.Error(1)
	}
	return nil, args.Error(1)
}
func (m *mockServiceSourceRepo) Insert(ctx context.Context, doc *model.GatewaySource) error {
	return m.Called(ctx, doc).Error(0)
}
func (m *mockServiceSourceRepo) Update(ctx context.Context, id bson.ObjectID, fields bson.M) error {
	return m.Called(ctx, id, fields).Error(0)
}
func (m *mockServiceSourceRepo) SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	return m.Called(ctx, id, deletedAt, deletedBy).Error(0)
}

func newSvc(repo *mockGatewayServiceRepo, srcRepo *mockServiceSourceRepo) *GatewayServiceService {
	return NewGatewayServiceService(repo, srcRepo, NewGatewayRedisSync(nil, 0, nil))
}

// errMock is a sentinel for repo failures
var errMock = errors.New("mock error")

const (
	testBasePath    = "/my-service"
	testSourceAlias = "my-source"
	testNewBasePath = "/new-path"
	testParamPathID = "/a/{id}"
)

// --- GetByID ---

func TestGatewayServiceServiceGetByID(t *testing.T) {
	ctx := context.Background()
	svcID := bson.NewObjectID()

	t.Run("พบ service คืน service", func(t *testing.T) {
		repo := new(mockGatewayServiceRepo)
		repo.On("FindByID", ctx, svcID).Return(&model.GatewayService{ID: svcID}, nil)

		result, err := newSvc(repo, new(mockServiceSourceRepo)).GetByID(ctx, svcID.Hex())

		require.NoError(t, err)
		assert.Equal(t, svcID, result.ID)
	})

	t.Run("ID ไม่ใช่ hex คืน ErrGatewayServiceNotFound", func(t *testing.T) {
		_, err := newSvc(new(mockGatewayServiceRepo), new(mockServiceSourceRepo)).GetByID(ctx, "bad-id")
		assert.ErrorIs(t, err, ErrGatewayServiceNotFound)
	})

	t.Run("ไม่พบใน DB คืน ErrGatewayServiceNotFound", func(t *testing.T) {
		repo := new(mockGatewayServiceRepo)
		repo.On("FindByID", ctx, svcID).Return(nil, errMock)

		_, err := newSvc(repo, new(mockServiceSourceRepo)).GetByID(ctx, svcID.Hex())

		assert.ErrorIs(t, err, ErrGatewayServiceNotFound)
	})
}

// --- Create ---

func TestGatewayServiceServiceCreate(t *testing.T) {
	ctx := context.Background()
	admin := &model.User{ID: bson.NewObjectID(), Role: "admin"}

	t.Run("ไม่มี resourcePaths สร้างสำเร็จ", func(t *testing.T) {
		repo := new(mockGatewayServiceRepo)
		repo.On("FindByBasePath", ctx, testBasePath).Return(nil, errMock)
		repo.On("Insert", ctx, mock.Anything).Return(nil)

		result, err := newSvc(repo, new(mockServiceSourceRepo)).Create(ctx, &model.GatewayServiceCreate{
			Name: "svc", Type: "General", BasePath: testBasePath,
		}, admin)

		require.NoError(t, err)
		assert.Equal(t, testBasePath, result.BasePath)
		assert.True(t, *result.Enabled)
		assert.Equal(t, admin.ID, *result.CreatedBy)
	})

	t.Run("enabled=false ใน body ถูกบันทึก", func(t *testing.T) {
		repo := new(mockGatewayServiceRepo)
		repo.On("FindByBasePath", ctx, testBasePath).Return(nil, errMock)
		repo.On("Insert", ctx, mock.Anything).Return(nil)

		disabled := false
		result, err := newSvc(repo, new(mockServiceSourceRepo)).Create(ctx, &model.GatewayServiceCreate{
			Name: "svc", Type: "General", BasePath: testBasePath, Enabled: &disabled,
		}, admin)

		require.NoError(t, err)
		assert.False(t, *result.Enabled)
	})

	t.Run("มี resourcePath ที่ sourceAlias มีใน DB สร้างสำเร็จ", func(t *testing.T) {
		repo := new(mockGatewayServiceRepo)
		srcRepo := new(mockServiceSourceRepo)
		existingSrc := &model.GatewaySource{ID: bson.NewObjectID(), Alias: testSourceAlias}
		repo.On("FindByBasePath", ctx, testBasePath).Return(nil, errMock)
		srcRepo.On("FindByAlias", ctx, testSourceAlias).Return(existingSrc, nil)
		repo.On("Insert", ctx, mock.Anything).Return(nil)

		result, err := newSvc(repo, srcRepo).Create(ctx, &model.GatewayServiceCreate{
			Name:     "svc",
			Type:     "General",
			BasePath: testBasePath,
			ResourcePaths: []*model.ResourcePath{
				{Path: "/items", Methods: []string{"GET"}, SourceAlias: testSourceAlias},
			},
		}, admin)

		require.NoError(t, err)
		assert.Equal(t, testSourceAlias, result.ResourcePaths[0].SourceAlias)
	})

	t.Run("มี inline source พร้อม alias สร้าง source ใหม่ด้วย", func(t *testing.T) {
		repo := new(mockGatewayServiceRepo)
		srcRepo := new(mockServiceSourceRepo)
		repo.On("FindByBasePath", ctx, testBasePath).Return(nil, errMock)
		srcRepo.On("FindByAlias", ctx, "new-alias").Return(nil, errMock)
		srcRepo.On("Insert", ctx, mock.Anything).Return(nil)
		repo.On("Insert", ctx, mock.Anything).Return(nil)

		result, err := newSvc(repo, srcRepo).Create(ctx, &model.GatewayServiceCreate{
			Name:     "svc",
			Type:     "General",
			BasePath: testBasePath,
			ResourcePaths: []*model.ResourcePath{
				{Path: "/items", SourceAlias: "new-alias", Source: &model.ResourcePathSourceInput{
					Name: "Backend", Type: "upstream", Protocol: "https", URL: "https://example.com",
				}},
			},
		}, admin)

		require.NoError(t, err)
		assert.Equal(t, "new-alias", result.ResourcePaths[0].SourceAlias)
		srcRepo.AssertCalled(t, "Insert", ctx, mock.Anything)
	})

	t.Run("inline source ไม่ใส่ alias — gen alias อัตโนมัติ", func(t *testing.T) {
		repo := new(mockGatewayServiceRepo)
		srcRepo := new(mockServiceSourceRepo)
		repo.On("FindByBasePath", ctx, testBasePath).Return(nil, errMock)
		srcRepo.On("FindByAlias", ctx, mock.AnythingOfType("string")).Return(nil, errMock)
		srcRepo.On("Insert", ctx, mock.Anything).Return(nil)
		repo.On("Insert", ctx, mock.Anything).Return(nil)

		rp := &model.ResourcePath{Path: "/items", Source: &model.ResourcePathSourceInput{
			Name: "Backend", Type: "upstream",
		}}
		_, err := newSvc(repo, srcRepo).Create(ctx, &model.GatewayServiceCreate{
			Name:          "svc",
			Type:          "General",
			BasePath:      testBasePath,
			ResourcePaths: []*model.ResourcePath{rp},
		}, admin)

		require.NoError(t, err)
		assert.NotEmpty(t, rp.SourceAlias, "ต้อง gen alias ใส่กลับใน rp")
		assert.Contains(t, rp.SourceAlias, "src-")
	})

	t.Run("inline source ใส่ alias ที่มีใน DB แล้ว — update source เดิม ไม่ error ไม่ Insert source ใหม่", func(t *testing.T) {
		existingSrcID := bson.NewObjectID()
		existingSrc := &model.GatewaySource{ID: existingSrcID, Alias: "dup-alias"}
		repo := new(mockGatewayServiceRepo)
		srcRepo := new(mockServiceSourceRepo)
		repo.On("FindByBasePath", ctx, testBasePath).Return(nil, errMock)
		srcRepo.On("FindByAlias", ctx, "dup-alias").Return(existingSrc, nil)
		srcRepo.On("Update", ctx, existingSrcID, mock.Anything).Return(nil)
		repo.On("Insert", ctx, mock.AnythingOfType("*model.GatewayService")).Return(nil)

		result, err := newSvc(repo, srcRepo).Create(ctx, &model.GatewayServiceCreate{
			Name:     "svc",
			Type:     "General",
			BasePath: testBasePath,
			ResourcePaths: []*model.ResourcePath{
				{Path: "/items", SourceAlias: "dup-alias", Source: &model.ResourcePathSourceInput{Name: "B", Type: "upstream", URL: "https://example.com"}},
			},
		}, admin)

		require.NoError(t, err)
		require.NotNil(t, result)
		srcRepo.AssertCalled(t, "Update", ctx, existingSrcID, mock.Anything)
		srcRepo.AssertNotCalled(t, "Insert")
	})

	t.Run("sourceAlias ไม่มีใน DB (ไม่มี inline source) — คืน error พร้อม field details", func(t *testing.T) {
		repo := new(mockGatewayServiceRepo)
		srcRepo := new(mockServiceSourceRepo)
		repo.On("FindByBasePath", ctx, testBasePath).Return(nil, errMock)
		srcRepo.On("FindByAlias", ctx, "ghost").Return(nil, errMock)

		_, err := newSvc(repo, srcRepo).Create(ctx, &model.GatewayServiceCreate{
			Name:     "svc",
			Type:     "General",
			BasePath: testBasePath,
			ResourcePaths: []*model.ResourcePath{
				{Path: "/items", SourceAlias: "ghost"},
			},
		}, admin)

		require.Error(t, err)
		var exc *model.Exception
		require.ErrorAs(t, err, &exc)
		assert.Equal(t, 400, exc.Status)
		props := exc.Properties
		fields, _ := props["fields"].(map[string]interface{})
		assert.Contains(t, fields, "resourcePaths[0].sourceAlias")
	})

	t.Run("basePath ซ้ำ คืน ErrGatewayServiceBasePathDuplicate", func(t *testing.T) {
		repo := new(mockGatewayServiceRepo)
		repo.On("FindByBasePath", ctx, testBasePath).Return(&model.GatewayService{BasePath: testBasePath}, nil)

		_, err := newSvc(repo, new(mockServiceSourceRepo)).Create(ctx, &model.GatewayServiceCreate{
			Name: "svc", Type: "General", BasePath: testBasePath,
		}, admin)

		assert.ErrorIs(t, err, ErrGatewayServiceBasePathDuplicate)
	})

	t.Run("resourcePaths ชนกันภายใน คืน ErrGatewayServiceResourcePathConflict", func(t *testing.T) {
		repo := new(mockGatewayServiceRepo)
		srcRepo := new(mockServiceSourceRepo)
		existingSrc := &model.GatewaySource{ID: bson.NewObjectID(), Alias: testSourceAlias}
		repo.On("FindByBasePath", ctx, testBasePath).Return(nil, errMock)
		srcRepo.On("FindByAlias", ctx, testSourceAlias).Return(existingSrc, nil)

		_, err := newSvc(repo, srcRepo).Create(ctx, &model.GatewayServiceCreate{
			Name:     "svc",
			Type:     "General",
			BasePath: testBasePath,
			ResourcePaths: []*model.ResourcePath{
				{Path: "/a/{id}", SourceAlias: testSourceAlias},
				{Path: "/a/{name}", SourceAlias: testSourceAlias},
			},
		}, admin)

		assert.ErrorIs(t, err, ErrGatewayServiceResourcePathConflict)
	})
}

// --- Update ---

func TestGatewayServiceServiceUpdate(t *testing.T) {
	ctx := context.Background()
	svcID := bson.NewObjectID()
	admin := &model.User{ID: bson.NewObjectID(), Role: "admin"}
	existingSvc := &model.GatewayService{ID: svcID, BasePath: "/old"}

	t.Run("อัปเดตสำเร็จ basePath ไม่เปลี่ยน", func(t *testing.T) {
		repo := new(mockGatewayServiceRepo)
		plain := &model.GatewayService{ID: svcID, Name: "New", BasePath: "/old"}
		withSource := &model.GatewayServiceWithSource{ID: svcID, Name: "New", BasePath: "/old"}
		repo.On("FindByID", ctx, svcID).Return(existingSvc, nil).Once()
		repo.On("Update", ctx, svcID, mock.Anything).Return(nil)
		repo.On("FindByID", ctx, svcID).Return(plain, nil).Once()
		repo.On("FindByIDWithSource", ctx, svcID).Return(withSource, nil)

		result, err := newSvc(repo, new(mockServiceSourceRepo)).Update(ctx, svcID.Hex(), &model.GatewayServiceUpdate{
			Name: "New", Type: "General", BasePath: "/old",
		}, admin)

		require.NoError(t, err)
		assert.Equal(t, "New", result.Name)
	})

	t.Run("basePath เปลี่ยนและซ้ำ คืน ErrGatewayServiceBasePathDuplicate", func(t *testing.T) {
		repo := new(mockGatewayServiceRepo)
		repo.On("FindByID", ctx, svcID).Return(existingSvc, nil)
		repo.On("FindByBasePath", ctx, testNewBasePath).Return(&model.GatewayService{ID: bson.NewObjectID()}, nil)

		_, err := newSvc(repo, new(mockServiceSourceRepo)).Update(ctx, svcID.Hex(), &model.GatewayServiceUpdate{
			Name: "x", Type: "General", BasePath: testNewBasePath,
		}, admin)

		assert.ErrorIs(t, err, ErrGatewayServiceBasePathDuplicate)
	})

	t.Run("resourcePaths ชนกันภายใน คืน ErrGatewayServiceResourcePathConflict", func(t *testing.T) {
		repo := new(mockGatewayServiceRepo)
		srcRepo := new(mockServiceSourceRepo)
		src := &model.GatewaySource{ID: bson.NewObjectID(), Alias: testSourceAlias}
		repo.On("FindByID", ctx, svcID).Return(existingSvc, nil)
		srcRepo.On("FindByAlias", ctx, testSourceAlias).Return(src, nil)

		_, err := newSvc(repo, srcRepo).Update(ctx, svcID.Hex(), &model.GatewayServiceUpdate{
			Name:     "x",
			Type:     "General",
			BasePath: "/old",
			ResourcePaths: []*model.ResourcePath{
				{Path: "/a/{id}", SourceAlias: testSourceAlias},
				{Path: "/a/{name}", SourceAlias: testSourceAlias},
			},
		}, admin)

		assert.ErrorIs(t, err, ErrGatewayServiceResourcePathConflict)
	})

	t.Run("sourceAlias ไม่มีใน DB — คืน error พร้อม field details", func(t *testing.T) {
		repo := new(mockGatewayServiceRepo)
		srcRepo := new(mockServiceSourceRepo)
		repo.On("FindByID", ctx, svcID).Return(existingSvc, nil)
		srcRepo.On("FindByAlias", ctx, "ghost").Return(nil, errMock)

		_, err := newSvc(repo, srcRepo).Update(ctx, svcID.Hex(), &model.GatewayServiceUpdate{
			Name:     "x",
			Type:     "General",
			BasePath: "/old",
			ResourcePaths: []*model.ResourcePath{
				{Path: "/items", SourceAlias: "ghost"},
			},
		}, admin)

		require.Error(t, err)
		var exc *model.Exception
		require.ErrorAs(t, err, &exc)
		assert.Equal(t, 400, exc.Status)
	})

	t.Run("alias หายออกจาก resourcePaths — source ถูก soft delete ถ้าไม่มี service อื่นใช้", func(t *testing.T) {
		removedAlias := "removed-src"
		srcID := bson.NewObjectID()
		existingWithSrc := &model.GatewayService{
			ID:       svcID,
			BasePath: "/old",
			ResourcePaths: []*model.ResourcePath{
				{Path: "/a", SourceAlias: removedAlias},
			},
		}
		repo := new(mockGatewayServiceRepo)
		srcRepo := new(mockServiceSourceRepo)
		plain := &model.GatewayService{ID: svcID, BasePath: "/old"}
		withSource := &model.GatewayServiceWithSource{ID: svcID, BasePath: "/old"}
		repo.On("FindByID", ctx, svcID).Return(existingWithSrc, nil).Once()
		repo.On("Update", ctx, svcID, mock.Anything).Return(nil)
		repo.On("FindByID", ctx, svcID).Return(plain, nil).Once()
		repo.On("FindByIDWithSource", ctx, svcID).Return(withSource, nil)
		repo.On("CountBySourceAlias", ctx, removedAlias).Return(int64(0), nil)
		srcRepo.On("FindByAlias", ctx, removedAlias).Return(&model.GatewaySource{ID: srcID, Alias: removedAlias}, nil)
		srcRepo.On("SoftDelete", ctx, srcID, mock.Anything, admin.ID).Return(nil)

		_, err := newSvc(repo, srcRepo).Update(ctx, svcID.Hex(), &model.GatewayServiceUpdate{
			Name: "x", Type: "General", BasePath: "/old",
		}, admin)

		require.NoError(t, err)
		srcRepo.AssertCalled(t, "SoftDelete", ctx, srcID, mock.Anything, admin.ID)
	})

	t.Run("alias หายแต่ยังมี service อื่นใช้ — source ไม่ถูก soft delete", func(t *testing.T) {
		removedAlias := "shared-src"
		existingWithSrc := &model.GatewayService{
			ID:       svcID,
			BasePath: "/old",
			ResourcePaths: []*model.ResourcePath{
				{Path: "/a", SourceAlias: removedAlias},
			},
		}
		repo := new(mockGatewayServiceRepo)
		srcRepo := new(mockServiceSourceRepo)
		plain := &model.GatewayService{ID: svcID, BasePath: "/old"}
		withSource := &model.GatewayServiceWithSource{ID: svcID, BasePath: "/old"}
		repo.On("FindByID", ctx, svcID).Return(existingWithSrc, nil).Once()
		repo.On("Update", ctx, svcID, mock.Anything).Return(nil)
		repo.On("FindByID", ctx, svcID).Return(plain, nil).Once()
		repo.On("FindByIDWithSource", ctx, svcID).Return(withSource, nil)
		repo.On("CountBySourceAlias", ctx, removedAlias).Return(int64(1), nil)
		srcRepo.On("FindByAlias", ctx, removedAlias).Return(&model.GatewaySource{Alias: removedAlias}, nil)

		_, err := newSvc(repo, srcRepo).Update(ctx, svcID.Hex(), &model.GatewayServiceUpdate{
			Name: "x", Type: "General", BasePath: "/old",
		}, admin)

		require.NoError(t, err)
		srcRepo.AssertNotCalled(t, "SoftDelete")
	})

	t.Run("inline source ที่ alias มีอยู่แล้ว — update source แทนการสร้างใหม่", func(t *testing.T) {
		existingSrcID := bson.NewObjectID()
		existingSrc := &model.GatewaySource{ID: existingSrcID, Alias: testSourceAlias, Name: "Old", URL: ""}
		plain := &model.GatewayService{ID: svcID, BasePath: "/old"}
		withSource := &model.GatewayServiceWithSource{ID: svcID, BasePath: "/old"}
		repo := new(mockGatewayServiceRepo)
		srcRepo := new(mockServiceSourceRepo)
		repo.On("FindByID", ctx, svcID).Return(existingSvc, nil).Once()
		srcRepo.On("FindByAlias", ctx, testSourceAlias).Return(existingSrc, nil)
		srcRepo.On("Update", ctx, existingSrcID, mock.Anything).Return(nil)
		repo.On("Update", ctx, svcID, mock.Anything).Return(nil)
		repo.On("FindByID", ctx, svcID).Return(plain, nil).Once()
		repo.On("FindByIDWithSource", ctx, svcID).Return(withSource, nil)

		_, err := newSvc(repo, srcRepo).Update(ctx, svcID.Hex(), &model.GatewayServiceUpdate{
			Name:     "x",
			Type:     "General",
			BasePath: "/old",
			ResourcePaths: []*model.ResourcePath{
				{Path: "/items", SourceAlias: testSourceAlias, Source: &model.ResourcePathSourceInput{
					Name: "New Name", Type: "upstream", Protocol: "https", URL: "https://example.com",
				}},
			},
		}, admin)

		require.NoError(t, err)
		srcRepo.AssertCalled(t, "Update", ctx, existingSrcID, mock.Anything)
		srcRepo.AssertNotCalled(t, "Insert")
	})

	t.Run("ไม่พบ service คืน ErrGatewayServiceNotFound", func(t *testing.T) {
		repo := new(mockGatewayServiceRepo)
		repo.On("FindByID", ctx, svcID).Return(nil, errMock)

		_, err := newSvc(repo, new(mockServiceSourceRepo)).Update(ctx, svcID.Hex(), &model.GatewayServiceUpdate{
			Name: "x", Type: "General", BasePath: "/x",
		}, admin)

		assert.ErrorIs(t, err, ErrGatewayServiceNotFound)
	})

	t.Run("ID ไม่ใช่ hex คืน ErrGatewayServiceNotFound", func(t *testing.T) {
		_, err := newSvc(new(mockGatewayServiceRepo), new(mockServiceSourceRepo)).Update(ctx, "bad-id", &model.GatewayServiceUpdate{
			Name: "x", Type: "General", BasePath: "/x",
		}, admin)

		assert.ErrorIs(t, err, ErrGatewayServiceNotFound)
	})
}

// --- Delete ---

func TestGatewayServiceServiceDelete(t *testing.T) {
	ctx := context.Background()
	svcID := bson.NewObjectID()
	admin := &model.User{ID: bson.NewObjectID()}

	t.Run("soft delete service สำเร็จ ไม่มี source ผูกอยู่", func(t *testing.T) {
		repo := new(mockGatewayServiceRepo)
		repo.On("FindByID", ctx, svcID).Return(&model.GatewayService{ID: svcID}, nil)
		repo.On("SoftDelete", ctx, svcID, mock.Anything, admin.ID).Return(nil)

		err := newSvc(repo, new(mockServiceSourceRepo)).Delete(ctx, svcID.Hex(), false, admin)

		require.NoError(t, err)
		repo.AssertCalled(t, "SoftDelete", ctx, svcID, mock.Anything, admin.ID)
	})

	t.Run("hard delete service สำเร็จ ไม่มี source ผูกอยู่", func(t *testing.T) {
		repo := new(mockGatewayServiceRepo)
		repo.On("FindByID", ctx, svcID).Return(&model.GatewayService{ID: svcID}, nil)
		repo.On("HardDelete", ctx, svcID).Return(nil)

		err := newSvc(repo, new(mockServiceSourceRepo)).Delete(ctx, svcID.Hex(), true, admin)

		require.NoError(t, err)
		repo.AssertCalled(t, "HardDelete", ctx, svcID)
	})

	t.Run("delete service ที่มี source ผูกอยู่ — source ถูก soft delete ถ้าไม่มี service อื่นใช้", func(t *testing.T) {
		alias := "sole-source"
		srcID := bson.NewObjectID()
		repo := new(mockGatewayServiceRepo)
		srcRepo := new(mockServiceSourceRepo)
		repo.On("FindByID", ctx, svcID).Return(&model.GatewayService{
			ID: svcID,
			ResourcePaths: []*model.ResourcePath{
				{Path: "/a", SourceAlias: alias},
			},
		}, nil)
		// count=1 คือ service นี้เอง (ก่อน delete)
		repo.On("CountBySourceAlias", ctx, alias).Return(int64(1), nil)
		srcRepo.On("FindByAlias", ctx, alias).Return(&model.GatewaySource{ID: srcID, Alias: alias}, nil)
		srcRepo.On("SoftDelete", ctx, srcID, mock.Anything, admin.ID).Return(nil)
		repo.On("SoftDelete", ctx, svcID, mock.Anything, admin.ID).Return(nil)

		err := newSvc(repo, srcRepo).Delete(ctx, svcID.Hex(), false, admin)

		require.NoError(t, err)
		srcRepo.AssertCalled(t, "SoftDelete", ctx, srcID, mock.Anything, admin.ID)
	})

	t.Run("delete service ที่ source ถูกใช้โดย service อื่นด้วย — source ไม่ถูก soft delete", func(t *testing.T) {
		alias := "shared-source"
		repo := new(mockGatewayServiceRepo)
		srcRepo := new(mockServiceSourceRepo)
		repo.On("FindByID", ctx, svcID).Return(&model.GatewayService{
			ID: svcID,
			ResourcePaths: []*model.ResourcePath{
				{Path: "/a", SourceAlias: alias},
			},
		}, nil)
		// count=2 หมายความว่ายังมี service อื่นใช้อยู่
		repo.On("CountBySourceAlias", ctx, alias).Return(int64(2), nil)
		srcRepo.On("FindByAlias", ctx, alias).Return(&model.GatewaySource{Alias: alias}, nil)
		repo.On("SoftDelete", ctx, svcID, mock.Anything, admin.ID).Return(nil)

		err := newSvc(repo, srcRepo).Delete(ctx, svcID.Hex(), false, admin)

		require.NoError(t, err)
		srcRepo.AssertNotCalled(t, "SoftDelete")
	})

	t.Run("ไม่พบ service คืน ErrGatewayServiceNotFound", func(t *testing.T) {
		repo := new(mockGatewayServiceRepo)
		repo.On("FindByID", ctx, svcID).Return(nil, errMock)

		err := newSvc(repo, new(mockServiceSourceRepo)).Delete(ctx, svcID.Hex(), false, admin)

		assert.ErrorIs(t, err, ErrGatewayServiceNotFound)
	})

	t.Run("ID ไม่ใช่ hex คืน ErrGatewayServiceNotFound", func(t *testing.T) {
		err := newSvc(new(mockGatewayServiceRepo), new(mockServiceSourceRepo)).Delete(ctx, "bad-id", false, admin)
		assert.ErrorIs(t, err, ErrGatewayServiceNotFound)
	})
}

// --- path conflict pure functions ---

func TestPathsConflict(t *testing.T) {
	cases := []struct {
		a, b     string
		conflict bool
	}{
		{"/a/b", "/a/b", true},
		{"/a/b", "/a/c", false},
		{"/a/{id}", "/a/b", false},
		{"/a/{id}", "/a/{name}", true},
		{"/a/{id}/items", "/a/{name}/items", true},
		{"/a/{id}/items", "/a/{name}/other", false},
		{"/a/*", "/a/b", false},
		{"/a/*", "/a/{id}", false},
		{"/a/{id}/b", "/a/x/c", false},
	}

	for _, tc := range cases {
		assert.Equal(t, tc.conflict, pathsConflict(tc.a, tc.b), "pathsConflict(%q, %q)", tc.a, tc.b)
	}
}

func TestCheckInternalPathConflicts(t *testing.T) {
	t.Run("ไม่มี conflict คืน nil", func(t *testing.T) {
		assert.Empty(t, checkInternalPathConflicts([]string{"/a", "/b", "/a/{id}"}))
	})

	t.Run("path เหมือนกัน คืน 1 group", func(t *testing.T) {
		result := checkInternalPathConflicts([]string{"/a", "/a"})
		require.Len(t, result, 1)
		assert.Equal(t, []string{"/a", "/a"}, result[0].ConflictingPaths)
	})

	t.Run("param normalized เหมือนกัน คืน 1 group", func(t *testing.T) {
		result := checkInternalPathConflicts([]string{"/a/{id}", "/a/{name}"})
		require.Len(t, result, 1)
		assert.Len(t, result[0].ConflictingPaths, 2)
	})

	t.Run("path ซ้ำหลายตัว group รวมกัน", func(t *testing.T) {
		result := checkInternalPathConflicts([]string{"/a", "/b", "/a", "/a"})
		require.Len(t, result, 1)
		assert.Len(t, result[0].ConflictingPaths, 3)
	})

	t.Run("หลาย pattern ที่ conflict คืนหลาย group", func(t *testing.T) {
		result := checkInternalPathConflicts([]string{"/a", "/a", "/b/{id}", "/b/{name}"})
		assert.Len(t, result, 2)
	})
}

// --- CheckPaths ---

func TestGatewayServiceServiceCheckPaths(t *testing.T) {
	ctx := context.Background()

	t.Run("ไม่มี conflict คืน hasConflict=false", func(t *testing.T) {
		repo := new(mockGatewayServiceRepo)
		repo.On("FindByBasePath", ctx, "/svc").Return(nil, errMock)

		resp, err := newSvc(repo, new(mockServiceSourceRepo)).CheckPaths(ctx, "/svc", []string{"/a", "/b"}, nil)

		require.NoError(t, err)
		assert.False(t, resp.HasConflict)
	})

	t.Run("basePath ซ้ำ คืน hasConflict=true พร้อม basePathConflict", func(t *testing.T) {
		repo := new(mockGatewayServiceRepo)
		conflictID := bson.NewObjectID()
		repo.On("FindByBasePath", ctx, "/svc").Return(&model.GatewayService{ID: conflictID, BasePath: "/svc"}, nil)

		resp, err := newSvc(repo, new(mockServiceSourceRepo)).CheckPaths(ctx, "/svc", []string{"/a"}, nil)

		require.NoError(t, err)
		assert.True(t, resp.HasConflict)
		require.NotNil(t, resp.BasePathConflict)
		assert.Equal(t, conflictID.Hex(), resp.BasePathConflict.ConflictingServiceID)
	})

	t.Run("basePath ซ้ำกับตัวเองและมี excludeID — ไม่ conflict", func(t *testing.T) {
		repo := new(mockGatewayServiceRepo)
		ownID := bson.NewObjectID()
		repo.On("FindByBasePath", ctx, "/svc").Return(&model.GatewayService{ID: ownID, BasePath: "/svc"}, nil)

		resp, err := newSvc(repo, new(mockServiceSourceRepo)).CheckPaths(ctx, "/svc", []string{"/a"}, &ownID)

		require.NoError(t, err)
		assert.False(t, resp.HasConflict)
	})

	t.Run("resourcePaths ชนกันภายใน คืน hasConflict=true พร้อม resourcePathConflicts", func(t *testing.T) {
		repo := new(mockGatewayServiceRepo)
		repo.On("FindByBasePath", ctx, "/svc").Return(nil, errMock)

		resp, err := newSvc(repo, new(mockServiceSourceRepo)).CheckPaths(ctx, "/svc", []string{"/a/{id}", "/a/{name}"}, nil)

		require.NoError(t, err)
		assert.True(t, resp.HasConflict)
		assert.Len(t, resp.ResourcePathConflicts, 1)
	})
}

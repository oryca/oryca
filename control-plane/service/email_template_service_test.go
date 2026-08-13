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

// --- mock repo ---
type mockEmailTemplateRepo struct {
	mock.Mock
}

func (m *mockEmailTemplateRepo) FindAll(ctx context.Context, params url.Values) ([]*model.EmailTemplate, int64, error) {
	args := m.Called(ctx, params)
	if v := args.Get(0); v != nil {
		return v.([]*model.EmailTemplate), args.Get(1).(int64), args.Error(2)
	}
	return nil, 0, args.Error(2)
}

func (m *mockEmailTemplateRepo) FindByID(ctx context.Context, id bson.ObjectID) (*model.EmailTemplate, error) {
	args := m.Called(ctx, id)
	if v := args.Get(0); v != nil {
		return v.(*model.EmailTemplate), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockEmailTemplateRepo) FindByAlias(ctx context.Context, alias string) (*model.EmailTemplate, error) {
	args := m.Called(ctx, alias)
	if v := args.Get(0); v != nil {
		return v.(*model.EmailTemplate), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockEmailTemplateRepo) Insert(ctx context.Context, doc *model.EmailTemplate) error {
	return m.Called(ctx, doc).Error(0)
}

func (m *mockEmailTemplateRepo) Update(ctx context.Context, id bson.ObjectID, fields bson.M) error {
	return m.Called(ctx, id, fields).Error(0)
}

func (m *mockEmailTemplateRepo) UnsetDefault(ctx context.Context, templateType string) error {
	return m.Called(ctx, templateType).Error(0)
}

func (m *mockEmailTemplateRepo) SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error {
	return m.Called(ctx, id, deletedAt).Error(0)
}

func (m *mockEmailTemplateRepo) HardDelete(ctx context.Context, id bson.ObjectID) error {
	return m.Called(ctx, id).Error(0)
}

// --- GetByID ---
func TestEmailTemplateService_GetByID(t *testing.T) {
	ctx := context.Background()
	id := bson.NewObjectID()

	t.Run("พบ template คืน template", func(t *testing.T) {
		repo := new(mockEmailTemplateRepo)
		tmpl := &model.EmailTemplate{ID: id, Name: "Verify Email"}
		repo.On("FindByID", ctx, id).Return(tmpl, nil)

		svc := NewEmailTemplateService(repo)
		result, err := svc.GetByID(ctx, id)

		require.NoError(t, err)
		assert.Equal(t, id, result.ID)
	})

	t.Run("ไม่พบ template คืน error", func(t *testing.T) {
		repo := new(mockEmailTemplateRepo)
		repo.On("FindByID", ctx, id).Return(nil, errors.New("not found"))

		svc := NewEmailTemplateService(repo)
		result, err := svc.GetByID(ctx, id)

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

// --- GetByAlias ---
func TestEmailTemplateService_GetByAlias(t *testing.T) {
	ctx := context.Background()

	t.Run("พบ template คืน template", func(t *testing.T) {
		repo := new(mockEmailTemplateRepo)
		tmpl := &model.EmailTemplate{Alias: "oryca.verify-email"}
		repo.On("FindByAlias", ctx, "oryca.verify-email").Return(tmpl, nil)

		svc := NewEmailTemplateService(repo)
		result, err := svc.GetByAlias(ctx, "oryca.verify-email")

		require.NoError(t, err)
		assert.Equal(t, "oryca.verify-email", result.Alias)
	})

	t.Run("ไม่พบ template คืน error", func(t *testing.T) {
		repo := new(mockEmailTemplateRepo)
		repo.On("FindByAlias", ctx, "unknown").Return(nil, errors.New("not found"))

		svc := NewEmailTemplateService(repo)
		result, err := svc.GetByAlias(ctx, "unknown")

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

// --- List ---
func TestEmailTemplateService_List(t *testing.T) {
	ctx := context.Background()
	params := url.Values{}

	t.Run("มีข้อมูล ต้องได้ list จาก repo", func(t *testing.T) {
		repo := new(mockEmailTemplateRepo)
		list := []*model.EmailTemplate{{Name: "Verify Email"}}
		repo.On("FindAll", ctx, params).Return(list, int64(1), nil)

		svc := NewEmailTemplateService(repo)
		result, total, err := svc.List(ctx, params)

		require.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, int64(1), total)
	})

	t.Run("repo error ต้อง return error", func(t *testing.T) {
		repo := new(mockEmailTemplateRepo)
		repo.On("FindAll", ctx, params).Return(nil, int64(0), errors.New("db error"))

		svc := NewEmailTemplateService(repo)
		result, _, err := svc.List(ctx, params)

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

// --- Create ---
func TestEmailTemplateService_Create(t *testing.T) {
	ctx := context.Background()

	t.Run("สร้าง non-default สำเร็จ ต้องไม่แตะ UnsetDefault", func(t *testing.T) {
		repo := new(mockEmailTemplateRepo)
		body := &model.EmailTemplateCreate{
			Name:    "Welcome",
			Alias:   "welcome",
			Type:    "notification",
			Default: false,
		}
		repo.On("Insert", ctx, mock.Anything).Return(nil)

		svc := NewEmailTemplateService(repo)
		result, err := svc.Create(ctx, body)

		require.NoError(t, err)
		assert.Equal(t, "Welcome", result.Name)
		assert.False(t, result.System)
		repo.AssertNotCalled(t, "UnsetDefault", mock.Anything, mock.Anything)
	})

	t.Run("สร้าง default ต้อง UnsetDefault ใน type เดียวกัน", func(t *testing.T) {
		repo := new(mockEmailTemplateRepo)
		body := &model.EmailTemplateCreate{
			Name:    "Custom Verify",
			Alias:   "custom.verify-email",
			Type:    "verify-email",
			Default: true,
		}
		repo.On("UnsetDefault", ctx, "verify-email").Return(nil)
		repo.On("Insert", ctx, mock.Anything).Return(nil)

		svc := NewEmailTemplateService(repo)
		result, err := svc.Create(ctx, body)

		require.NoError(t, err)
		assert.True(t, result.Default)
		repo.AssertCalled(t, "UnsetDefault", ctx, "verify-email")
	})

	t.Run("UnsetDefault error ต้อง return error", func(t *testing.T) {
		repo := new(mockEmailTemplateRepo)
		body := &model.EmailTemplateCreate{Name: "x", Type: "verify-email", Default: true}
		repo.On("UnsetDefault", ctx, "verify-email").Return(errors.New("db error"))

		svc := NewEmailTemplateService(repo)
		result, err := svc.Create(ctx, body)

		assert.Error(t, err)
		assert.Nil(t, result)
		repo.AssertNotCalled(t, "Insert", mock.Anything, mock.Anything)
	})

	t.Run("Insert error ต้อง return error", func(t *testing.T) {
		repo := new(mockEmailTemplateRepo)
		body := &model.EmailTemplateCreate{Name: "x", Type: "notification", Default: false}
		repo.On("Insert", ctx, mock.Anything).Return(errors.New("write failed"))

		svc := NewEmailTemplateService(repo)
		result, err := svc.Create(ctx, body)

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

// --- Update ---
func TestEmailTemplateService_Update(t *testing.T) {
	ctx := context.Background()
	id := bson.NewObjectID()

	t.Run("อัปเดต non-default สำเร็จ ต้องไม่แตะ UnsetDefault", func(t *testing.T) {
		repo := new(mockEmailTemplateRepo)
		body := &model.EmailTemplateUpdate{Name: "Updated", Type: "notification", Default: false}
		updated := &model.EmailTemplate{ID: id, Name: "Updated"}

		repo.On("Update", ctx, id, mock.Anything).Return(nil)
		repo.On("FindByID", ctx, id).Return(updated, nil)

		svc := NewEmailTemplateService(repo)
		result, err := svc.Update(ctx, id, body)

		require.NoError(t, err)
		assert.Equal(t, "Updated", result.Name)
		repo.AssertNotCalled(t, "UnsetDefault", mock.Anything, mock.Anything)
	})

	t.Run("อัปเดตเป็น default ต้อง UnsetDefault ใน type เดียวกัน", func(t *testing.T) {
		repo := new(mockEmailTemplateRepo)
		body := &model.EmailTemplateUpdate{Name: "New Default", Type: "verify-email", Default: true}
		updated := &model.EmailTemplate{ID: id, Name: "New Default", Default: true}

		repo.On("UnsetDefault", ctx, "verify-email").Return(nil)
		repo.On("Update", ctx, id, mock.Anything).Return(nil)
		repo.On("FindByID", ctx, id).Return(updated, nil)

		svc := NewEmailTemplateService(repo)
		result, err := svc.Update(ctx, id, body)

		require.NoError(t, err)
		assert.True(t, result.Default)
		repo.AssertCalled(t, "UnsetDefault", ctx, "verify-email")
	})

	t.Run("repo.Update error ต้อง return error", func(t *testing.T) {
		repo := new(mockEmailTemplateRepo)
		body := &model.EmailTemplateUpdate{Name: "x", Type: "notification", Default: false}
		repo.On("Update", ctx, id, mock.Anything).Return(errors.New("write failed"))

		svc := NewEmailTemplateService(repo)
		result, err := svc.Update(ctx, id, body)

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

// --- Delete ---
func TestEmailTemplateService_Delete(t *testing.T) {
	ctx := context.Background()
	id := bson.NewObjectID()

	t.Run("soft delete สำเร็จ ต้องไม่เรียก HardDelete", func(t *testing.T) {
		repo := new(mockEmailTemplateRepo)
		tmpl := &model.EmailTemplate{ID: id, Name: "Welcome", System: false}

		repo.On("FindByID", ctx, id).Return(tmpl, nil)
		repo.On("SoftDelete", ctx, id, mock.AnythingOfType("time.Time")).Return(nil)

		svc := NewEmailTemplateService(repo)
		err := svc.Delete(ctx, id, false, bson.NilObjectID)

		require.NoError(t, err)
		repo.AssertNotCalled(t, "HardDelete", mock.Anything, mock.Anything)
	})

	t.Run("hard delete (forever=true) ต้องเรียก HardDelete", func(t *testing.T) {
		repo := new(mockEmailTemplateRepo)
		tmpl := &model.EmailTemplate{ID: id, Name: "Welcome", System: false}

		repo.On("FindByID", ctx, id).Return(tmpl, nil)
		repo.On("HardDelete", ctx, id).Return(nil)

		svc := NewEmailTemplateService(repo)
		err := svc.Delete(ctx, id, true, bson.NilObjectID)

		require.NoError(t, err)
		repo.AssertNotCalled(t, "SoftDelete", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("ลบ system template ต้อง return ErrDeleteSystemEmailTemplate", func(t *testing.T) {
		repo := new(mockEmailTemplateRepo)
		tmpl := &model.EmailTemplate{ID: id, Alias: "oryca.verify-email", System: true}

		repo.On("FindByID", ctx, id).Return(tmpl, nil)

		svc := NewEmailTemplateService(repo)
		err := svc.Delete(ctx, id, false, bson.NilObjectID)

		assert.ErrorIs(t, err, ErrDeleteSystemEmailTemplate)
		repo.AssertNotCalled(t, "SoftDelete", mock.Anything, mock.Anything, mock.Anything)
		repo.AssertNotCalled(t, "HardDelete", mock.Anything, mock.Anything)
	})

	t.Run("ลบ system template แบบ forever ก็ยังต้อง return ErrDeleteSystemEmailTemplate", func(t *testing.T) {
		repo := new(mockEmailTemplateRepo)
		tmpl := &model.EmailTemplate{ID: id, Alias: "oryca.reset-password", System: true}

		repo.On("FindByID", ctx, id).Return(tmpl, nil)

		svc := NewEmailTemplateService(repo)
		err := svc.Delete(ctx, id, true, bson.NilObjectID)

		assert.ErrorIs(t, err, ErrDeleteSystemEmailTemplate)
		repo.AssertNotCalled(t, "HardDelete", mock.Anything, mock.Anything)
	})

	t.Run("ไม่พบ template ต้อง return error", func(t *testing.T) {
		repo := new(mockEmailTemplateRepo)
		repo.On("FindByID", ctx, id).Return(nil, errors.New("not found"))

		svc := NewEmailTemplateService(repo)
		err := svc.Delete(ctx, id, false, bson.NilObjectID)

		assert.Error(t, err)
	})
}

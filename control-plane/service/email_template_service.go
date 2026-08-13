package service

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/tool"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ErrDeleteSystemEmailTemplate คืนเมื่อพยายามลบ template ที่เป็น system (oryca built-in)
var ErrDeleteSystemEmailTemplate = errors.New("cannot delete system email template")

type emailTemplateRepo interface {
	FindAll(ctx context.Context, params url.Values) ([]*model.EmailTemplate, int64, error)
	FindByID(ctx context.Context, id bson.ObjectID) (*model.EmailTemplate, error)
	FindByAlias(ctx context.Context, alias string) (*model.EmailTemplate, error)
	Insert(ctx context.Context, doc *model.EmailTemplate) error
	Update(ctx context.Context, id bson.ObjectID, fields bson.M) error
	UnsetDefault(ctx context.Context, templateType string) error
	SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error
	HardDelete(ctx context.Context, id bson.ObjectID) error
}

type EmailTemplateService struct {
	repo emailTemplateRepo
}

func NewEmailTemplateService(repo emailTemplateRepo) *EmailTemplateService {
	return &EmailTemplateService{repo: repo}
}

func (s *EmailTemplateService) List(ctx context.Context, params url.Values) ([]*model.EmailTemplate, int64, error) {
	return s.repo.FindAll(ctx, params)
}

func (s *EmailTemplateService) GetByID(ctx context.Context, id bson.ObjectID) (*model.EmailTemplate, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *EmailTemplateService) GetByAlias(ctx context.Context, alias string) (*model.EmailTemplate, error) {
	return s.repo.FindByAlias(ctx, alias)
}

func (s *EmailTemplateService) Create(ctx context.Context, body *model.EmailTemplateCreate) (*model.EmailTemplate, error) {
	// ใน type เดียวกัน default ได้แค่ 1 ตัว
	if body.Default {
		if err := s.repo.UnsetDefault(ctx, body.Type); err != nil {
			return nil, err
		}
	}

	now := tool.NowUTC()
	doc := &model.EmailTemplate{
		ID:        bson.NewObjectID(),
		Name:      body.Name,
		Alias:     body.Alias,
		Type:      body.Type,
		Default:   body.Default,
		System:    false,
		Subject:   body.Subject,
		Variables: body.Variables,
		HtmlBody:  body.HtmlBody,
		CreatedAt: &now,
	}

	if err := s.repo.Insert(ctx, doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func (s *EmailTemplateService) Update(ctx context.Context, id bson.ObjectID, body *model.EmailTemplateUpdate) (*model.EmailTemplate, error) {
	// ใน type เดียวกัน default ได้แค่ 1 ตัว
	if body.Default {
		if err := s.repo.UnsetDefault(ctx, body.Type); err != nil {
			return nil, err
		}
	}

	now := tool.NowUTC()
	fields := bson.M{
		"name":      body.Name,
		"type":      body.Type,
		"default":   body.Default,
		"subject":   body.Subject,
		"variables": body.Variables,
		"htmlBody":  body.HtmlBody,
		"updatedAt": now,
	}

	if err := s.repo.Update(ctx, id, fields); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, id)
}

// Delete ลบ template แบบ soft (default) หรือ hard ถ้า forever=true
// ไม่อนุญาตให้ลบ system template (oryca built-in)
func (s *EmailTemplateService) Delete(ctx context.Context, id bson.ObjectID, forever bool, deletedBy bson.ObjectID) error {
	tmpl, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}

	if tmpl.System {
		return ErrDeleteSystemEmailTemplate
	}

	if forever {
		return s.repo.HardDelete(ctx, id)
	}
	return s.repo.SoftDelete(ctx, id, tool.NowUTC(), deletedBy)
}

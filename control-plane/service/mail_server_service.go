package service

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/tool"
	"go.mongodb.org/mongo-driver/v2/bson"
	"golang.org/x/sync/singleflight"
)

// ErrDeleteDefaultMailServer คืนเมื่อพยายามลบ mail server ที่เป็น default
var ErrDeleteDefaultMailServer = errors.New("cannot delete default mail server")

type mailServerRepo interface {
	FindAll(ctx context.Context, params url.Values) ([]*model.MailServer, int64, error)
	FindByID(ctx context.Context, id bson.ObjectID) (*model.MailServer, error)
	FindDefault(ctx context.Context) (*model.MailServer, error)
	Insert(ctx context.Context, doc *model.MailServer) error
	Update(ctx context.Context, id bson.ObjectID, doc *model.MailServer) error
	UnsetDefault(ctx context.Context) error
	SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error
	HardDelete(ctx context.Context, id bson.ObjectID) error
}

type mailServerCache interface {
	Get(ctx context.Context) (*model.MailServer, error)
	Set(ctx context.Context, ms *model.MailServer) error
	Delete(ctx context.Context) error
	SF() *singleflight.Group
}

type MailServerService struct {
	repo  mailServerRepo
	cache mailServerCache
}

func NewMailServerService(repo mailServerRepo, cache mailServerCache) *MailServerService {
	return &MailServerService{repo: repo, cache: cache}
}

func (s *MailServerService) List(ctx context.Context, params url.Values) ([]*model.MailServer, int64, error) {
	return s.repo.FindAll(ctx, params)
}

func (s *MailServerService) GetByID(ctx context.Context, id bson.ObjectID) (*model.MailServer, error) {
	return s.repo.FindByID(ctx, id)
}

// GetDefault คืน default mail server โดยใช้ cache-first + singleflight
func (s *MailServerService) GetDefault(ctx context.Context) (*model.MailServer, error) {
	// ถ้ามีใน cache คืนค่าทันที
	cached, err := s.cache.Get(ctx)
	if err == nil && cached != nil {
		return cached, nil
	}

	// cache miss — singleflight ป้องกันไม่ให้ยิง DB ซ้ำซ้อน
	// request ที่เข้ามาพร้อมกันจะ block รอผลจาก request แรกแทน
	v, err, _ := s.cache.SF().Do("get_default_mail_server", func() (any, error) {
		ms, err := s.repo.FindDefault(ctx)
		if err != nil {
			return nil, err
		}
		_ = s.cache.Set(ctx, ms) // cache error ไม่ถือเป็น fatal
		return ms, nil
	})
	if err != nil {
		return nil, err
	}

	return v.(*model.MailServer), nil
}

func (s *MailServerService) Update(ctx context.Context, id bson.ObjectID, body *model.MailServerUpdate) (*model.MailServer, error) {
	// ระบบมี default mail server ได้แค่ 1 ตัวเท่านั้น
	// ถ้า set default=true ต้องล้าง default ของตัวเก่าก่อน
	if body.Default {
		if err := s.repo.UnsetDefault(ctx); err != nil {
			return nil, err
		}
	}

	// ล้าง cache ทุกครั้งที่ update เพื่อให้ได้ข้อมูลล่าสุดเสมอ
	_ = s.cache.Delete(ctx)

	now := tool.NowUTC()
	doc := &model.MailServer{
		Name:                 body.Name,
		Default:              body.Default,
		Sender:               body.Sender,
		SenderEmail:          body.SenderEmail,
		Auth:                 body.Auth,
		Smtp:                 body.Smtp,
		ResetPasswordExpired: body.ResetPasswordExpired,
		VerifyEmailExpired:   body.VerifyEmailExpired,
		UpdatedAt:            &now,
	}

	if err := s.repo.Update(ctx, id, doc); err != nil {
		return nil, err
	}

	updated, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// write-through cache เฉพาะ default
	if updated.Default {
		_ = s.cache.Set(ctx, updated)
	}

	return updated, nil
}

// Delete ลบ mail server แบบ soft (default) หรือ hard ถ้า forever=true
// ไม่อนุญาตให้ลบ mail server ที่เป็น default
func (s *MailServerService) Delete(ctx context.Context, id bson.ObjectID, forever bool, deletedBy bson.ObjectID) error {
	ms, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}

	if ms.Default {
		return ErrDeleteDefaultMailServer
	}

	if forever {
		return s.repo.HardDelete(ctx, id)
	}

	return s.repo.SoftDelete(ctx, id, tool.NowUTC(), deletedBy)
}

func (s *MailServerService) Create(ctx context.Context, body *model.MailServerCreate) (*model.MailServer, error) {
	// ระบบมี default mail server ได้แค่ 1 ตัวเท่านั้น
	// ถ้า set default=true ต้องล้าง default ของตัวเก่าก่อน
	// ถ้า default=false ไม่ต้องแตะ record อื่น
	if body.Default {
		if err := s.repo.UnsetDefault(ctx); err != nil {
			return nil, err
		}
		// ล้าง cache เดิมก่อน set ใหม่
		_ = s.cache.Delete(ctx)
	}

	now := tool.NowUTC()
	doc := &model.MailServer{
		ID:                   bson.NewObjectID(),
		Name:                 body.Name,
		Default:              body.Default,
		Sender:               body.Sender,
		SenderEmail:          body.SenderEmail,
		Auth:                 body.Auth,
		Smtp:                 body.Smtp,
		ResetPasswordExpired: body.ResetPasswordExpired,
		VerifyEmailExpired:   body.VerifyEmailExpired,
		CreatedAt:            &now,
	}

	if err := s.repo.Insert(ctx, doc); err != nil {
		return nil, err
	}

	// write-through cache เฉพาะ default
	if body.Default {
		_ = s.cache.Set(ctx, doc)
	}

	return doc, nil
}

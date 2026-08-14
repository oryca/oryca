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

// ErrDeleteDefaultMailServer is returned when something tries to delete the default mail server
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

// GetDefault returns the default mail server, from the cache first, behind singleflight
func (s *MailServerService) GetDefault(ctx context.Context) (*model.MailServer, error) {
	// a hit answers straight away
	cached, err := s.cache.Get(ctx)
	if err == nil && cached != nil {
		return cached, nil
	}

	// on a miss, singleflight keeps the database to one query: everything arriving at the same time
	// waits on the first request instead
	v, err, _ := s.cache.SF().Do("get_default_mail_server", func() (any, error) {
		ms, err := s.repo.FindDefault(ctx)
		if err != nil {
			return nil, err
		}
		_ = s.cache.Set(ctx, ms) // a cache error is not fatal
		return ms, nil
	})
	if err != nil {
		return nil, err
	}

	return v.(*model.MailServer), nil
}

func (s *MailServerService) Update(ctx context.Context, id bson.ObjectID, body *model.MailServerUpdate) (*model.MailServer, error) {
	// there can only be one default mail server, so setting default=true has to clear the old one
	if body.Default {
		if err := s.repo.UnsetDefault(ctx); err != nil {
			return nil, err
		}
	}

	// drop the cache on every update, so the next read is current
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

	// only the default is written through to the cache
	if updated.Default {
		_ = s.cache.Set(ctx, updated)
	}

	return updated, nil
}

// Delete removes a mail server, softly by default and for good when forever=true.
// The default mail server cannot be deleted.
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
	// there can only be one default mail server, so setting default=true has to clear the old one
	// default=false leaves every other record alone
	if body.Default {
		if err := s.repo.UnsetDefault(ctx); err != nil {
			return nil, err
		}
		// clear the old entry before setting the new one
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

	// only the default is written through to the cache
	if body.Default {
		_ = s.cache.Set(ctx, doc)
	}

	return doc, nil
}

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

var (
	ErrGatewaySourceNotFound       = errors.New("gateway source not found")
	ErrGatewaySourceAliasDuplicate = errors.New("gateway source alias already exists")
	ErrGatewaySourceAliasInUse     = errors.New("gateway source alias is in use by a service")
)

type gatewaySourceRepo interface {
	FindAll(ctx context.Context, params url.Values) ([]*model.GatewaySource, int64, error)
	FindAllActive(ctx context.Context) ([]*model.GatewaySource, error)
	FindByID(ctx context.Context, id bson.ObjectID) (*model.GatewaySource, error)
	FindByAlias(ctx context.Context, alias string) (*model.GatewaySource, error)
	FindByIDOrAlias(ctx context.Context, idOrAlias string) (*model.GatewaySource, error)
	Insert(ctx context.Context, doc *model.GatewaySource) error
	Update(ctx context.Context, id bson.ObjectID, fields bson.M) error
	SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error
	HardDelete(ctx context.Context, id bson.ObjectID) error
}

type gatewaySourceInUseChecker interface {
	CountBySourceAlias(ctx context.Context, alias string) (int64, error)
}

type GatewaySourceService struct {
	repo        gatewaySourceRepo
	serviceRepo gatewaySourceInUseChecker
	redisSync   *GatewayRedisSync
}

func NewGatewaySourceService(repo gatewaySourceRepo, serviceRepo gatewaySourceInUseChecker, redisSync *GatewayRedisSync) *GatewaySourceService {
	return &GatewaySourceService{repo: repo, serviceRepo: serviceRepo, redisSync: redisSync}
}

func (s *GatewaySourceService) List(ctx context.Context, params url.Values) ([]*model.GatewaySource, int64, error) {
	return s.repo.FindAll(ctx, params)
}

// ListAllActive returns every non-deleted source with no pagination cap — used for boot-time
// Redis resync where a partial page would silently leave some sources unsynced.
func (s *GatewaySourceService) ListAllActive(ctx context.Context) ([]*model.GatewaySource, error) {
	return s.repo.FindAllActive(ctx)
}

func (s *GatewaySourceService) GetByIDOrAlias(ctx context.Context, idOrAlias string) (*model.GatewaySource, error) {
	src, err := s.repo.FindByIDOrAlias(ctx, idOrAlias)
	if err != nil {
		return nil, ErrGatewaySourceNotFound
	}
	return src, nil
}

func (s *GatewaySourceService) Create(ctx context.Context, body *model.GatewaySourceCreate, ctxUser *model.User) (*model.GatewaySource, error) {
	if existing, err := s.repo.FindByAlias(ctx, body.Alias); err == nil && existing != nil {
		return nil, ErrGatewaySourceAliasDuplicate
	}

	now := tool.NowUTC()
	doc := &model.GatewaySource{
		ID:          bson.NewObjectID(),
		Alias:       body.Alias,
		Name:        body.Name,
		Description: body.Description,
		Type:        body.Type,
		Protocol:    body.Protocol,
		URL:         body.URL,
		Headers:     body.Headers,
		ContentType: body.ContentType,
		Body:        body.Body,
		CreatedAt:   &now,
		CreatedBy:   &ctxUser.ID,
		UpdatedAt:   &now,
		UpdatedBy:   &ctxUser.ID,
	}

	if err := s.repo.Insert(ctx, doc); err != nil {
		return nil, err
	}

	_ = s.redisSync.SyncSource(ctx, doc)
	return doc, nil
}

func (s *GatewaySourceService) Update(ctx context.Context, idOrAlias string, body *model.GatewaySourceUpdate, ctxUser *model.User) (*model.GatewaySource, error) {
	existing, err := s.repo.FindByIDOrAlias(ctx, idOrAlias)
	if err != nil {
		return nil, ErrGatewaySourceNotFound
	}

	if body.Alias != existing.Alias {
		if dup, err := s.repo.FindByAlias(ctx, body.Alias); err == nil && dup != nil {
			return nil, ErrGatewaySourceAliasDuplicate
		}
	}

	now := tool.NowUTC()
	fields := bson.M{
		"updatedAt":   now,
		"updatedBy":   ctxUser.ID,
		"alias":       body.Alias,
		"name":        body.Name,
		"description": body.Description,
		"type":        body.Type,
		"protocol":    body.Protocol,
		"url":         body.URL,
		"headers":     body.Headers,
		"contentType": body.ContentType,
		"body":        body.Body,
	}

	if err := s.repo.Update(ctx, existing.ID, fields); err != nil {
		return nil, err
	}

	updated, err := s.repo.FindByID(ctx, existing.ID)
	if err != nil {
		return nil, err
	}
	_ = s.redisSync.SyncSource(ctx, updated)
	return updated, nil
}

func (s *GatewaySourceService) Delete(ctx context.Context, idOrAlias string, forever bool, ctxUser *model.User) error {
	existing, err := s.repo.FindByIDOrAlias(ctx, idOrAlias)
	if err != nil {
		return ErrGatewaySourceNotFound
	}

	count, err := s.serviceRepo.CountBySourceAlias(ctx, existing.Alias)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrGatewaySourceAliasInUse
	}

	if forever {
		if err := s.repo.HardDelete(ctx, existing.ID); err != nil {
			return err
		}
		_ = s.redisSync.DeleteSource(ctx, existing.Alias)
		return nil
	}

	now := tool.NowUTC()
	if err := s.repo.SoftDelete(ctx, existing.ID, now, ctxUser.ID); err != nil {
		return err
	}
	_ = s.redisSync.DeleteSource(ctx, existing.Alias)
	return nil
}

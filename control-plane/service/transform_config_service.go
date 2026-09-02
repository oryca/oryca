package service

import (
	"context"
	"errors"

	"github.com/oryca/oryca/control-plane/model"

	"go.mongodb.org/mongo-driver/v2/bson"
)

var ErrTransformConfigNotFound = errors.New("transform config not found")

type transformConfigRepo interface {
	Create(ctx context.Context, cfg *model.TransformConfig, adminID bson.ObjectID) error
	FindByID(ctx context.Context, id bson.ObjectID) (*model.TransformConfig, error)
	List(ctx context.Context, serviceID *bson.ObjectID, limit, offset int64) ([]*model.TransformConfig, int64, error)
	Update(ctx context.Context, id bson.ObjectID, req *model.TransformConfigRequest, adminID bson.ObjectID) error
	Delete(ctx context.Context, id bson.ObjectID, adminID bson.ObjectID) error
}

// transformGatewayPublisher tells the gateway to re-read its config. Transform
// configs are edited one at a time by an administrator, so there is no bulk loop
// here that could turn into an event storm.
type transformGatewayPublisher interface {
	Publish(eventType string, payload any)
}

type TransformResponseConfigService struct {
	repo      transformConfigRepo
	publisher transformGatewayPublisher
}

func NewTransformConfigService(repo transformConfigRepo, publisher transformGatewayPublisher) *TransformResponseConfigService {
	return &TransformResponseConfigService{repo: repo, publisher: publisher}
}

// notifyGateway asks the gateway to re-pull. Without it a new transform waits for
// the next poll, which makes a saved change look like it did nothing.
func (s *TransformResponseConfigService) notifyGateway() {
	if s.publisher != nil {
		s.publisher.Publish(SyncEventTypeReloadServices, nil)
	}
}

func (s *TransformResponseConfigService) List(ctx context.Context, serviceID *bson.ObjectID, limit, offset int64) ([]*model.TransformConfig, int64, error) {
	return s.repo.List(ctx, serviceID, limit, offset)
}

func (s *TransformResponseConfigService) GetByID(ctx context.Context, id bson.ObjectID) (*model.TransformConfig, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *TransformResponseConfigService) Create(ctx context.Context, req *model.TransformConfigRequest, adminID bson.ObjectID) (*model.TransformConfig, error) {
	serviceID, err := bson.ObjectIDFromHex(req.ServiceID)
	if err != nil {
		return nil, err
	}
	cfg := &model.TransformConfig{
		Name:        req.Name,
		Description: req.Description,
		ServiceID:   serviceID,
		Match:       req.Match,
		Enabled:     req.Enabled,
		Rules:       req.Rules,
	}
	if err := s.repo.Create(ctx, cfg, adminID); err != nil {
		return nil, err
	}
	s.notifyGateway()
	return cfg, nil
}

func (s *TransformResponseConfigService) Update(ctx context.Context, id bson.ObjectID, req *model.TransformConfigRequest, adminID bson.ObjectID) error {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrTransformConfigNotFound
	}
	if err := s.repo.Update(ctx, id, req, adminID); err != nil {
		return err
	}
	s.notifyGateway()
	return nil
}

func (s *TransformResponseConfigService) Delete(ctx context.Context, id bson.ObjectID, adminID bson.ObjectID) error {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrTransformConfigNotFound
	}
	if err := s.repo.Delete(ctx, id, adminID); err != nil {
		return err
	}
	s.notifyGateway()
	return nil
}

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

type TransformResponseConfigService struct {
	repo transformConfigRepo
}

func NewTransformConfigService(repo transformConfigRepo) *TransformResponseConfigService {
	return &TransformResponseConfigService{repo: repo}
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
	return s.repo.Update(ctx, id, req, adminID)
}

func (s *TransformResponseConfigService) Delete(ctx context.Context, id bson.ObjectID, adminID bson.ObjectID) error {
	existing, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrTransformConfigNotFound
	}
	return s.repo.Delete(ctx, id, adminID)
}

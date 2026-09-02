package service

import (
	"context"
	"errors"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/tool"

	"go.mongodb.org/mongo-driver/v2/bson"
)

var ErrGatewaySpecNotFound = errors.New("gateway spec not found")

type gatewaySpecRepo interface {
	FindByServiceID(ctx context.Context, serviceID bson.ObjectID) (*model.GatewaySpec, error)
	Insert(ctx context.Context, doc *model.GatewaySpec) error
	Update(ctx context.Context, id bson.ObjectID, fields bson.M) error
	Delete(ctx context.Context, serviceID bson.ObjectID) error
}

type GatewaySpecService struct {
	repo gatewaySpecRepo
}

func NewGatewaySpecService(repo gatewaySpecRepo) *GatewaySpecService {
	return &GatewaySpecService{repo: repo}
}

func (s *GatewaySpecService) GetByServiceID(ctx context.Context, serviceID string) (*model.GatewaySpec, error) {
	oid, err := bson.ObjectIDFromHex(serviceID)
	if err != nil {
		return nil, ErrGatewaySpecNotFound
	}
	spec, err := s.repo.FindByServiceID(ctx, oid)
	if err != nil {
		return nil, ErrGatewaySpecNotFound
	}
	return spec, nil
}

func (s *GatewaySpecService) Upsert(ctx context.Context, serviceID string, title string, description string, body *model.GatewaySpecUpsert, ctxUser *model.User) (*model.GatewaySpec, error) {
	oid, err := bson.ObjectIDFromHex(serviceID)
	if err != nil {
		return nil, ErrGatewaySpecNotFound
	}

	now := tool.NowUTC()
	existing, err := s.repo.FindByServiceID(ctx, oid)
	if err != nil {
		doc := &model.GatewaySpec{
			ID:                bson.NewObjectID(),
			ServiceID:         oid,
			Title:             title,
			Description:       description,
			Version:           body.Version,
			ServerDescription: body.ServerDescription,
			Paths:             body.Paths,
			CreatedAt:         &now,
			CreatedBy:         &ctxUser.ID,
			UpdatedAt:         &now,
			UpdatedBy:         &ctxUser.ID,
		}
		if err := s.repo.Insert(ctx, doc); err != nil {
			return nil, err
		}
		return doc, nil
	}

	fields := bson.M{
		"title":             title,
		"description":       description,
		"version":           body.Version,
		"serverDescription": body.ServerDescription,
		"paths":             body.Paths,
		"updatedAt":         now,
		"updatedBy":         ctxUser.ID,
	}
	if err := s.repo.Update(ctx, existing.ID, fields); err != nil {
		return nil, err
	}
	return s.repo.FindByServiceID(ctx, oid)
}

func (s *GatewaySpecService) Delete(ctx context.Context, serviceID string) error {
	oid, err := bson.ObjectIDFromHex(serviceID)
	if err != nil {
		return ErrGatewaySpecNotFound
	}
	if _, err := s.repo.FindByServiceID(ctx, oid); err != nil {
		return ErrGatewaySpecNotFound
	}
	return s.repo.Delete(ctx, oid)
}

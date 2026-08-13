package service

import (
	"context"
	"errors"
	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/tool"
	"net/url"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

var (
	ErrApiDocNotFound      = errors.New("api doc not found")
	ErrApiDocFailedReFetch = errors.New("api doc failed to re-fetch")
)

type apiDocRepo interface {
	FindAll(ctx context.Context, params url.Values) ([]*model.ApiDoc, int64, error)
	FindByID(ctx context.Context, id bson.ObjectID, params *url.Values) (*model.ApiDoc, error)
	Insert(ctx context.Context, doc *model.ApiDoc) error
	Update(ctx context.Context, id bson.ObjectID, fields bson.M) error
	SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error
	HardDelete(ctx context.Context, id bson.ObjectID) error
	FindPathsByID(ctx context.Context, id bson.ObjectID, apiDocParams, pathParams url.Values) ([]*model.ApiDocPath, int64, error)
}

type ApiDocService struct {
	repo apiDocRepo
}

func NewApiDocService(repo apiDocRepo) *ApiDocService {
	return &ApiDocService{repo: repo}
}

func (s *ApiDocService) List(ctx context.Context, params url.Values) ([]*model.ApiDoc, int64, error) {
	apidocs, total, err := s.repo.FindAll(ctx, params)
	if err != nil {
		return nil, 0, err
	}

	return apidocs, total, nil
}

func (s *ApiDocService) GetByID(ctx context.Context, id bson.ObjectID, params url.Values) (*model.ApiDoc, error) {
	apidoc, err := s.repo.FindByID(ctx, id, &params)
	if err != nil {
		return nil, ErrApiDocNotFound
	}

	return apidoc, nil
}

func (s *ApiDocService) Create(ctx context.Context, body *model.ApiDocCreate, ctxUser *model.User) (*model.ApiDoc, error) {

	now := tool.NowUTC()
	doc := &model.ApiDoc{
		ID:             bson.NewObjectID(),
		OpenAPI:        body.OpenAPI,
		AuthenRequired: body.AuthenRequired,
		PublishStatus:  body.PublishStatus,
		Info:           body.Info,
		Servers:        body.Servers,
		Tags:           body.Tags,
		Paths:          body.Paths,
		CreatedAt:      &now,
		CreatedBy:      &ctxUser.ID,
		UpdatedAt:      &now,
		UpdatedBy:      &ctxUser.ID,
	}

	err := s.repo.Insert(ctx, doc)
	if err != nil {
		return nil, err
	}

	return doc, nil
}

func (s *ApiDocService) Update(ctx context.Context, id bson.ObjectID, body *model.ApiDocUpdate, ctxUser *model.User) (*model.ApiDoc, error) {
	_, err := s.repo.FindByID(ctx, id, nil)
	if err != nil {
		return nil, ErrApiDocNotFound
	}

	now := tool.NowUTC()
	fields := bson.M{
		"openapi":        body.OpenAPI,
		"authenRequired": body.AuthenRequired,
		"publishStatus":  body.PublishStatus,
		"info":           body.Info,
		"servers":        body.Servers,
		"tags":           body.Tags,
		"paths":          body.Paths,
		"updatedAt":      &now,
		"updatedBy":      &ctxUser.ID,
	}
	err = s.repo.Update(ctx, id, fields)
	if err != nil {
		return nil, err
	}

	updated, err := s.repo.FindByID(ctx, id, nil)
	if err != nil {
		return nil, ErrApiDocFailedReFetch
	}

	return updated, nil
}

func (s *ApiDocService) Delete(ctx context.Context, id bson.ObjectID, forever bool, ctxUser *model.User) error {
	_, err := s.repo.FindByID(ctx, id, nil)
	if err != nil {
		return ErrApiDocNotFound
	}

	now := tool.NowUTC()
	if forever {
		err = s.repo.HardDelete(ctx, id)
	} else {
		err = s.repo.SoftDelete(ctx, id, now, ctxUser.ID)
	}
	if err != nil {
		return err
	}

	return nil
}

func (s *ApiDocService) GetPathsByID(ctx context.Context, id bson.ObjectID, params url.Values) ([]*model.ApiDocPath, int64, error) {

	apiDocParams := url.Values{}
	if v := params.Get("authenRequired"); v != "" {
		apiDocParams.Add("authenRequired", v)
		params.Del("authenRequired")
	}
	if v := params.Get("publishStatus"); v != "" {
		apiDocParams.Add("publishStatus", v)
		params.Del("publishStatus")
	}
	paths, total, err := s.repo.FindPathsByID(ctx, id, apiDocParams, params)
	if err != nil {
		return nil, 0, ErrApiDocNotFound
	}

	return paths, total, nil
}

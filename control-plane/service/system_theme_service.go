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

var (
	ErrSystemThemeNotFound      = errors.New("system theme not found")
	ErrDeleteDefaultSystemTheme = errors.New("cannot delete default system theme")
	ErrDefaultSystemThemeExists = errors.New("default system theme already exists")
)

type systemThemeRepo interface {
	FindAll(ctx context.Context, params url.Values) ([]*model.SystemTheme, int64, error)
	FindByID(ctx context.Context, id bson.ObjectID) (*model.SystemTheme, error)
	FindDefault(ctx context.Context) (*model.SystemTheme, error)
	Insert(ctx context.Context, doc *model.SystemTheme) error
	Update(ctx context.Context, id bson.ObjectID, fields bson.M) error
	UnsetDefault(ctx context.Context) error
	SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error
}

type systemThemeCache interface {
	Get(ctx context.Context) (*model.SystemTheme, error)
	Set(ctx context.Context, theme *model.SystemTheme) error
	Delete(ctx context.Context) error
	SF() *singleflight.Group
}

type SystemThemeService struct {
	repo  systemThemeRepo
	cache systemThemeCache
}

func NewSystemThemeService(repo systemThemeRepo, cache systemThemeCache) *SystemThemeService {
	return &SystemThemeService{repo: repo, cache: cache}
}

func (s *SystemThemeService) List(ctx context.Context, params url.Values) ([]*model.SystemTheme, int64, error) {
	return s.repo.FindAll(ctx, params)
}

func (s *SystemThemeService) GetByID(ctx context.Context, id bson.ObjectID) (*model.SystemTheme, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *SystemThemeService) GetDefault(ctx context.Context) (*model.SystemTheme, error) {
	cached, err := s.cache.Get(ctx)
	if err == nil && cached != nil {
		return cached, nil
	}

	v, err, _ := s.cache.SF().Do("get_default_system_theme", func() (any, error) {
		theme, err := s.repo.FindDefault(ctx)
		if err != nil {
			return nil, err
		}
		_ = s.cache.Set(ctx, theme)
		return theme, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*model.SystemTheme), nil
}

func (s *SystemThemeService) Create(ctx context.Context, body *model.SystemThemeCreate, createdBy bson.ObjectID) (*model.SystemTheme, error) {
	if body.Default {
		if existing, _ := s.repo.FindDefault(ctx); existing != nil {
			return nil, ErrDefaultSystemThemeExists
		}
	}

	now := tool.NowUTC()
	doc := &model.SystemTheme{
		ID:          bson.NewObjectID(),
		Name:        body.Name,
		Description: body.Description,
		Default:     body.Default,
		Logo:        body.Logo,
		Title:       body.Title,
		ThemeConfig: body.ThemeConfig,
		CreatedAt:   &now,
		CreatedBy:   &createdBy,
	}

	if err := s.repo.Insert(ctx, doc); err != nil {
		return nil, err
	}

	if body.Default {
		_ = s.cache.Set(ctx, doc)
	}
	return doc, nil
}

func (s *SystemThemeService) Update(ctx context.Context, id bson.ObjectID, body *model.SystemThemeUpdate, updatedBy bson.ObjectID) (*model.SystemTheme, error) {
	if body.Default {
		if existing, _ := s.repo.FindDefault(ctx); existing != nil && existing.ID != id {
			return nil, ErrDefaultSystemThemeExists
		}
	}

	now := tool.NowUTC()
	fields := bson.M{
		"name":        body.Name,
		"description": body.Description,
		"default":     body.Default,
		"logo":        body.Logo,
		"title":       body.Title,
		"themeConfig": body.ThemeConfig,
		"updatedAt":   now,
		"updatedBy":   updatedBy,
	}

	if err := s.repo.Update(ctx, id, fields); err != nil {
		return nil, err
	}

	updated, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if updated.Default {
		_ = s.cache.Set(ctx, updated)
	} else {
		_ = s.cache.Delete(ctx)
	}
	return updated, nil
}

func (s *SystemThemeService) Delete(ctx context.Context, id bson.ObjectID, deletedBy bson.ObjectID) error {
	theme, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return ErrSystemThemeNotFound
	}
	if theme.Default {
		return ErrDeleteDefaultSystemTheme
	}
	if err := s.repo.SoftDelete(ctx, id, tool.NowUTC(), deletedBy); err != nil {
		return err
	}
	_ = s.cache.Delete(ctx)
	return nil
}

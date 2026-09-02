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

// applyPolicyDefaults ตั้งค่า default สำหรับ rateLimit.algorithm
func applyPolicyDefaults(policies *model.PackagePolicies) {
	if policies == nil {
		return
	}
	if rl := policies.RateLimit; rl != nil && rl.Enabled && rl.Algorithm == "" {
		rl.Algorithm = "sliding_window"
	}
}

var (
	ErrPackageNotFound       = errors.New("package not found")
	ErrPackageAliasDuplicate = errors.New("package alias already exists")
)

type packageRepo interface {
	FindAll(ctx context.Context, params url.Values) ([]*model.Package, int64, error)
	FindByID(ctx context.Context, id bson.ObjectID) (*model.Package, error)
	FindByAlias(ctx context.Context, alias string) (*model.Package, error)
	Insert(ctx context.Context, doc *model.Package) error
	Update(ctx context.Context, id bson.ObjectID, fields bson.M) error
	SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error
	HardDelete(ctx context.Context, id bson.ObjectID) error
	IncrUserCount(ctx context.Context, id bson.ObjectID, delta int64) error
}

type PackageService struct {
	repo packageRepo
}

func NewPackageService(repo packageRepo) *PackageService {
	return &PackageService{repo: repo}
}

func (s *PackageService) List(ctx context.Context, params url.Values) ([]*model.Package, int64, error) {
	return s.repo.FindAll(ctx, params)
}

func (s *PackageService) GetByID(ctx context.Context, id string) (*model.Package, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, ErrPackageNotFound
	}
	pkg, err := s.repo.FindByID(ctx, oid)
	if err != nil {
		return nil, ErrPackageNotFound
	}
	return pkg, nil
}

func (s *PackageService) Create(ctx context.Context, body *model.PackageCreate, ctxUser *model.User) (*model.Package, error) {
	alias := body.Alias
	if alias == "" {
		for {
			token, err := tool.GenerateToken(8)
			if err != nil {
				return nil, err
			}
			existing, err := s.repo.FindByAlias(ctx, token)
			if err != nil || existing == nil {
				alias = token
				break
			}
		}
	} else {
		if existing, err := s.repo.FindByAlias(ctx, alias); err == nil && existing != nil {
			return nil, ErrPackageAliasDuplicate
		}
	}
	body.Alias = alias

	applyPolicyDefaults(body.Policies)

	now := tool.NowUTC()
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}

	doc := &model.Package{
		ID:          bson.NewObjectID(),
		Name:        body.Name,
		Alias:       body.Alias,
		Description: body.Description,
		Enabled:     &enabled,
		Policies:    body.Policies,
		Properties:  body.Properties,
		CreatedAt:   &now,
		CreatedBy:   &ctxUser.ID,
		UpdatedAt:   &now,
		UpdatedBy:   &ctxUser.ID,
	}

	if err := s.repo.Insert(ctx, doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func (s *PackageService) Update(ctx context.Context, id string, body *model.PackageUpdate, ctxUser *model.User) (*model.Package, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, ErrPackageNotFound
	}

	existing, err := s.repo.FindByID(ctx, oid)
	if err != nil {
		return nil, ErrPackageNotFound
	}

	if body.Alias != existing.Alias {
		if dup, err := s.repo.FindByAlias(ctx, body.Alias); err == nil && dup != nil {
			return nil, ErrPackageAliasDuplicate
		}
	}

	applyPolicyDefaults(body.Policies)

	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}

	now := tool.NowUTC()
	fields := bson.M{
		"updatedAt":   now,
		"updatedBy":   ctxUser.ID,
		"name":        body.Name,
		"alias":       body.Alias,
		"description": body.Description,
		"enabled":     enabled,
		"policies":    body.Policies,
		"properties":  body.Properties,
	}

	if err := s.repo.Update(ctx, existing.ID, fields); err != nil {
		return nil, err
	}
	return s.repo.FindByID(ctx, existing.ID)
}

func (s *PackageService) Delete(ctx context.Context, id string, forever bool, ctxUser *model.User) error {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return ErrPackageNotFound
	}

	existing, err := s.repo.FindByID(ctx, oid)
	if err != nil {
		return ErrPackageNotFound
	}

	if forever {
		return s.repo.HardDelete(ctx, existing.ID)
	}

	now := tool.NowUTC()
	return s.repo.SoftDelete(ctx, existing.ID, now, ctxUser.ID)
}

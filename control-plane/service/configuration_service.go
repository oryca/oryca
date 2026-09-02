package service

import (
	"context"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/tool"
	"golang.org/x/sync/singleflight"
)

type configurationRepo interface {
	Get(ctx context.Context) (*model.Configuration, error)
	Upsert(ctx context.Context, doc *model.Configuration) error
}

type configurationCache interface {
	Get(ctx context.Context) (*model.Configuration, error)
	Set(ctx context.Context, cfg *model.Configuration) error
	SF() *singleflight.Group
}

type ConfigurationService struct {
	repo  configurationRepo
	cache configurationCache
}

func NewConfigurationService(repo configurationRepo, cache configurationCache) *ConfigurationService {
	return &ConfigurationService{repo: repo, cache: cache}
}

func (s *ConfigurationService) Get(ctx context.Context) (*model.Configuration, error) {
	// ถ้ามีข้อมูลใน cache คืนค่าทันที
	cached, err := s.cache.Get(ctx)
	if err == nil && cached != nil {
		return cached, nil
	}

	// cache miss. Singleflight ป้องกันไม่ให้ยิง DB ซ้ำซ้อน
	// request ที่เข้ามาพร้อมกันจะ block รอผลจาก request แรกแทน
	v, err, _ := s.cache.SF().Do("get_configuration", func() (any, error) {
		cfg, err := s.repo.Get(ctx)
		if err != nil {
			return nil, err
		}
		_ = s.cache.Set(ctx, cfg) // cache error ไม่ถือเป็น fatal
		return cfg, nil
	})
	if err != nil {
		return nil, err
	}

	return v.(*model.Configuration), nil
}

func (s *ConfigurationService) Upsert(ctx context.Context, body *model.ConfigurationUpsert) (*model.Configuration, error) {
	now := tool.NowUTC()

	doc := &model.Configuration{
		Terms:         body.Terms,
		PrivacyPolicy: body.PrivacyPolicy,
		Token:         body.Token,
		Register:      body.Register,
		UpdatedAt:     &now,
	}

	if err := s.repo.Upsert(ctx, doc); err != nil {
		return nil, err
	}

	// ดึงข้อมูลจริงจาก DB หลัง upsert
	cfg, err := s.repo.Get(ctx)
	if err != nil {
		return nil, err
	}

	// อัปเดต cache ให้ตรงกับ DB ทันที (write-through)
	_ = s.cache.Set(ctx, cfg)

	return cfg, nil
}

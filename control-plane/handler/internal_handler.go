package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"strings"

	"github.com/oryca/oryca/control-plane/model"

	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type internalServiceRepo interface {
	FindAllActive(ctx context.Context) ([]*model.GatewayService, error)
	FindByID(ctx context.Context, id bson.ObjectID) (*model.GatewayService, error)
	FindByIDs(ctx context.Context, ids []bson.ObjectID) ([]*model.GatewayService, error)
}

type internalSourceRepo interface {
	FindAllActive(ctx context.Context) ([]*model.GatewaySource, error)
}

type internalApiKeyRepo interface {
	FindAllActive(ctx context.Context) ([]*model.ApiKey, error)
	FindByIDs(ctx context.Context, ids []bson.ObjectID) ([]*model.ApiKey, error)
}

type internalUserRepo interface {
	FindByID(ctx context.Context, id bson.ObjectID) (*model.User, error)
	FindByIDs(ctx context.Context, ids []bson.ObjectID) ([]*model.User, error)
}

type internalPackageRepo interface {
	FindByIDs(ctx context.Context, ids []bson.ObjectID) ([]*model.Package, error)
}

type internalPackageSvcLinkRepo interface {
	FindPackageIDsByServiceID(ctx context.Context, serviceID bson.ObjectID) ([]string, error)
	FindByServiceID(ctx context.Context, serviceID bson.ObjectID) ([]*model.PackageSvcLink, error)
}

type internalTransformConfigRepo interface {
	FindAllActive(ctx context.Context) ([]*model.TransformConfig, error)
}

type InternalHandlerConfig struct {
	ServiceRepo    internalServiceRepo
	SourceRepo     internalSourceRepo
	ApiKeyRepo     internalApiKeyRepo
	UserRepo       internalUserRepo
	PackageRepo    internalPackageRepo
	PackageSvcRepo internalPackageSvcLinkRepo
	TransformRepo  internalTransformConfigRepo
}

type InternalHandler struct {
	serviceRepo    internalServiceRepo
	sourceRepo     internalSourceRepo
	apiKeyRepo     internalApiKeyRepo
	userRepo       internalUserRepo
	packageRepo    internalPackageRepo
	packageSvcRepo internalPackageSvcLinkRepo
	transformRepo  internalTransformConfigRepo
}

func NewInternalHandler(cfg InternalHandlerConfig) *InternalHandler {
	return &InternalHandler{
		serviceRepo:    cfg.ServiceRepo,
		sourceRepo:     cfg.SourceRepo,
		apiKeyRepo:     cfg.ApiKeyRepo,
		userRepo:       cfg.UserRepo,
		packageRepo:    cfg.PackageRepo,
		packageSvcRepo: cfg.PackageSvcRepo,
		transformRepo:  cfg.TransformRepo,
	}
}

// respondWithETag คำนวณ ETag จาก JSON payload แล้วตอบ 304 ถ้า client มีของเดิม
func respondWithETag(c echo.Context, data []byte) error {
	h := fnv.New64a()
	h.Write(data)
	etag := fmt.Sprintf(`"%x"`, h.Sum64())

	c.Response().Header().Set("ETag", etag)
	if c.Request().Header.Get("If-None-Match") == etag {
		return c.NoContent(http.StatusNotModified)
	}
	return c.JSONBlob(http.StatusOK, data)
}

// GetServices returns all active services in gateway payload format
func (h *InternalHandler) GetServices(c echo.Context) error {
	svcs, err := h.serviceRepo.FindAllActive(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	result := make([]*gwServicePayload, 0, len(svcs))
	for _, svc := range svcs {
		links, _ := h.packageSvcRepo.FindByServiceID(c.Request().Context(), svc.ID)
		packageIDs, rateLimitMap := buildRateLimitMap(links)
		result = append(result, buildServicePayload(svc, packageIDs, rateLimitMap))
	}

	data, err := json.Marshal(result)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return respondWithETag(c, data)
}

type sourcePayload struct {
	Alias       string                  `json:"alias"`
	Type        string                  `json:"type"`
	Protocol    string                  `json:"protocol,omitempty"`
	URL         string                  `json:"url,omitempty"`
	Headers     []*model.SourceKeyValue `json:"headers,omitempty"`
	ContentType string                  `json:"contentType,omitempty"`
	Body        string                  `json:"body,omitempty"`
}

// GetSources returns all active sources in gateway payload format
func (h *InternalHandler) GetSources(c echo.Context) error {
	srcs, err := h.sourceRepo.FindAllActive(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	result := make([]*sourcePayload, 0, len(srcs))
	for _, src := range srcs {
		result = append(result, &sourcePayload{
			Alias:       src.Alias,
			Type:        src.Type,
			Protocol:    src.Protocol,
			URL:         src.URL,
			Headers:     src.Headers,
			ContentType: src.ContentType,
			Body:        src.Body,
		})
	}

	data, err := json.Marshal(result)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return respondWithETag(c, data)
}

// GetApiKeys returns all active api-keys with embedded owner info
func (h *InternalHandler) GetApiKeys(c echo.Context) error {
	ctx := c.Request().Context()
	keys, err := h.apiKeyRepo.FindAllActive(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// ดึง owner ทั้งหมดใน 1 query แทนการเรียก FindByID ทีละ api-key (N+1) —
	// ที่ 60k+ users/keys การ query ทีละคนคือ 60,000+ round-trip ต่อ request เดียว
	ownerByID, err := ownerMapForApiKeys(ctx, h.userRepo, keys)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	result := make([]*model.ApiKeyCache, 0, len(keys))
	for _, ak := range keys {
		payload := &model.ApiKeyCache{
			ID:          ak.ID.Hex(),
			ApiKey:      ak.ApiKey,
			Enabled:     ak.Enabled,
			ExpiredAt:   ak.ExpiredAt,
			Restriction: ak.Restriction,
		}

		if ak.OwnerBy != nil {
			if owner, ok := ownerByID[*ak.OwnerBy]; ok {
				packageID := ""
				if owner.PackageID != nil {
					packageID = owner.PackageID.Hex()
				}
				payload.Owner = &model.ApiKeyOwnerCache{
					ID:        owner.ID.Hex(),
					Role:      owner.Role,
					PackageID: packageID,
					Verified:  owner.Verified,
					Enabled:   owner.Enabled,
					ExpiredAt: owner.ExpiredAt,
				}
			}
		}

		result = append(result, payload)
	}

	data, err := json.Marshal(result)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return respondWithETag(c, data)
}

// ownerMapForApiKeys รวบรวม OwnerBy ID ที่ไม่ซ้ำกันจาก api-key ทั้งหมด แล้ว fetch owner
// ผ่าน FindByIDs ครั้งเดียว คืนเป็น map[id]*model.User สำหรับ lookup แบบ O(1) ต่อ key
func ownerMapForApiKeys(ctx context.Context, userRepo internalUserRepo, keys []*model.ApiKey) (map[bson.ObjectID]*model.User, error) {
	seen := make(map[bson.ObjectID]struct{})
	ids := make([]bson.ObjectID, 0, len(keys))
	for _, ak := range keys {
		if ak.OwnerBy == nil {
			continue
		}
		if _, dup := seen[*ak.OwnerBy]; dup {
			continue
		}
		seen[*ak.OwnerBy] = struct{}{}
		ids = append(ids, *ak.OwnerBy)
	}

	owners, err := userRepo.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	ownerByID := make(map[bson.ObjectID]*model.User, len(owners))
	for _, o := range owners {
		ownerByID[o.ID] = o
	}
	return ownerByID, nil
}

// ResolveNamesRequest รับ id หลายชนิดพร้อมกัน — ผู้เรียก (เช่น analytics) ส่ง id ที่ dedupe
// มาแล้วเข้ามาในคำขอเดียว แทนการยิงทีละ id หรือทีละชนิดหลายรอบ
type ResolveNamesRequest struct {
	UserIDs    []string `json:"userIds,omitempty"`
	ServiceIDs []string `json:"serviceIds,omitempty"`
	PackageIDs []string `json:"packageIds,omitempty"`
	ApiKeyIDs  []string `json:"apiKeyIds,omitempty"`
}

type ResolveNamesResponse struct {
	Users    map[string]string `json:"users,omitempty"`
	Services map[string]string `json:"services,omitempty"`
	Packages map[string]string `json:"packages,omitempty"`
	ApiKeys  map[string]string `json:"apiKeys,omitempty"`
}

// parseObjectIDs แปลง string เป็น bson.ObjectID โดย "ข้าม" ตัวที่ parse ไม่ได้แทนที่จะ fail
// ทั้ง request — id ตัวเดียวที่เพี้ยนไม่ควรทำให้ id อื่นๆ ที่ resolve ได้ปกติพลอยหายไปด้วย
func parseObjectIDs(raw []string) []bson.ObjectID {
	ids := make([]bson.ObjectID, 0, len(raw))
	for _, s := range raw {
		if id, err := bson.ObjectIDFromHex(s); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func userDisplayName(u *model.User) string {
	if u.DisplayName != "" {
		return u.DisplayName
	}
	if u.FirstName != "" || u.LastName != "" {
		return strings.TrimSpace(u.FirstName + " " + u.LastName)
	}
	if u.Username != "" {
		return u.Username
	}
	return u.Email
}

func (h *InternalHandler) resolveUserNames(ctx context.Context, rawIDs []string) (map[string]string, error) {
	ids := parseObjectIDs(rawIDs)
	if len(ids) == 0 {
		return nil, nil
	}
	users, err := h.userRepo.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(users))
	for _, u := range users {
		names[u.ID.Hex()] = userDisplayName(u)
	}
	return names, nil
}

func (h *InternalHandler) resolveServiceNames(ctx context.Context, rawIDs []string) (map[string]string, error) {
	ids := parseObjectIDs(rawIDs)
	if len(ids) == 0 {
		return nil, nil
	}
	svcs, err := h.serviceRepo.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(svcs))
	for _, s := range svcs {
		names[s.ID.Hex()] = s.Name
	}
	return names, nil
}

func (h *InternalHandler) resolvePackageNames(ctx context.Context, rawIDs []string) (map[string]string, error) {
	ids := parseObjectIDs(rawIDs)
	if len(ids) == 0 {
		return nil, nil
	}
	pkgs, err := h.packageRepo.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(pkgs))
	for _, p := range pkgs {
		names[p.ID.Hex()] = p.Name
	}
	return names, nil
}

func (h *InternalHandler) resolveApiKeyNames(ctx context.Context, rawIDs []string) (map[string]string, error) {
	ids := parseObjectIDs(rawIDs)
	if len(ids) == 0 {
		return nil, nil
	}
	keys, err := h.apiKeyRepo.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(keys))
	for _, k := range keys {
		names[k.ID.Hex()] = k.Name
	}
	return names, nil
}

// ResolveNames คืนชื่อของ user/service/package/api-key จาก id หลายตัวพร้อมกัน แบบ batch
// ($in query ต่อชนิด id ไม่ใช่ loop ทีละตัว) — ให้ผู้เรียก (เช่น analytics) resolve id→name
// ในคำขอเดียวแทนที่จะยิงกลับมาทีละตัวหรือทีละชนิด
func (h *InternalHandler) ResolveNames(c echo.Context) error {
	var req ResolveNamesRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	ctx := c.Request().Context()

	users, err := h.resolveUserNames(ctx, req.UserIDs)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	services, err := h.resolveServiceNames(ctx, req.ServiceIDs)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	packages, err := h.resolvePackageNames(ctx, req.PackageIDs)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	apiKeys, err := h.resolveApiKeyNames(ctx, req.ApiKeyIDs)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, ResolveNamesResponse{
		Users:    users,
		Services: services,
		Packages: packages,
		ApiKeys:  apiKeys,
	})
}

func (h *InternalHandler) GetTransformConfigs(c echo.Context) error {
	cfgs, err := h.transformRepo.FindAllActive(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if cfgs == nil {
		cfgs = []*model.TransformConfig{}
	}
	data, err := json.Marshal(cfgs)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return respondWithETag(c, data)
}

// GetUser returns the gateway-facing freshness snapshot for a single user — the
// on-demand lookup backing oryca-gateway's per-pod user cache. Deliberately a
// single-ID lookup, not a bulk list: the gateway fetches lazily per-user on cache
// miss, so this never needs to build a payload covering every user.
func (h *InternalHandler) GetUser(c echo.Context) error {
	id, err := bson.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid user id"})
	}
	u, err := h.userRepo.FindByID(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "user not found"})
	}
	return c.JSON(http.StatusOK, model.ToUserFreshnessCache(u))
}

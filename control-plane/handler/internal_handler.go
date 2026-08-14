package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"strings"

	"github.com/oryca/oryca/control-plane/logger"
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

// respondWithETag computes an ETag over the JSON payload and answers 304 when the client has it already
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
	allLinks := make([][]*model.PackageSvcLink, len(svcs))
	seen := make(map[bson.ObjectID]struct{})
	for i, svc := range svcs {
		links, _ := h.packageSvcRepo.FindByServiceID(c.Request().Context(), svc.ID)
		allLinks[i] = links
		for _, link := range links {
			seen[link.PackageID] = struct{}{}
		}
	}

	// Every package's own limit, fetched once, so a path without a limit of its own can fall back to it
	packageLimits := h.packageRateLimits(c.Request().Context(), seen)

	for i, svc := range svcs {
		packageIDs, rateLimitMap := buildRateLimitMap(allLinks[i], packageLimits)
		result = append(result, buildServicePayload(svc, packageIDs, rateLimitMap))
	}

	data, err := json.Marshal(result)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return respondWithETag(c, data)
}

// packageRateLimits reads the limit set on each package itself, keyed by package id. One query for
// all of them, whatever the number of services.
func (h *InternalHandler) packageRateLimits(ctx context.Context, ids map[bson.ObjectID]struct{}) map[string]*model.PackageRateLimit {
	limits := make(map[string]*model.PackageRateLimit, len(ids))
	if len(ids) == 0 {
		return limits
	}
	list := make([]bson.ObjectID, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	pkgs, err := h.packageRepo.FindByIDs(ctx, list)
	if err != nil {
		logger.Error("internal: could not read package rate limits: " + err.Error())
		return limits
	}
	for _, pkg := range pkgs {
		if pkg.Policies != nil && pkg.Policies.RateLimit != nil {
			limits[pkg.ID.Hex()] = pkg.Policies.RateLimit
		}
	}
	return limits
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

	// fetch every owner in one query, rather than a FindByID per api-key (N+1). At 60k users and keys,
	// one at a time means 60,000 round trips for a single request.
	ownerByID, err := ownerMapForApiKeys(ctx, h.userRepo, keys)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	result := make([]*model.ApiKeyCache, 0, len(keys))
	for _, ak := range keys {
		payload := &model.ApiKeyCache{
			ID:        ak.ID.Hex(),
			ApiKey:    ak.ApiKey,
			Enabled:   ak.Enabled,
			ExpiredAt: ak.ExpiredAt,
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

// ownerMapForApiKeys collects the distinct OwnerBy ids across every api-key, fetches them with one
// FindByIDs, and returns a map[id]*model.User so each key costs O(1) to resolve.
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

// ResolveNamesRequest takes ids of several kinds at once. A caller such as analytics sends its
// already-deduplicated ids in a single request, instead of one call per id or per kind.
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

// parseObjectIDs converts strings to bson.ObjectID, skipping any it cannot parse rather than
// failing the whole request: one malformed id should not cost the caller every other one.
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

// ResolveNames returns the names behind user, service, package and api-key ids, in one batch: an
// $in query per kind, not a loop per id. A caller such as analytics resolves every id to a name in
// a single request.
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

// GetUser returns the gateway-facing freshness snapshot for a single user. The
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

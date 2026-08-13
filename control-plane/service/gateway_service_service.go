package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/tool"

	"go.mongodb.org/mongo-driver/v2/bson"
)

var (
	ErrGatewayServiceNotFound             = errors.New("gateway service not found")
	ErrGatewayServiceBasePathDuplicate    = errors.New("gateway service basePath already exists")
	ErrGatewayServiceResourcePathConflict = errors.New("gateway service resource path conflicts internally")
	ErrGatewayServiceSourceAliasInvalid   = errors.New("one or more sourceAlias values do not exist")
	ErrGatewayServiceSourceAliasDuplicate = errors.New("one or more sourceAlias values already exist")
)

type gatewayServiceRepo interface {
	FindAll(ctx context.Context, params url.Values) ([]*model.GatewayService, int64, error)
	FindAllActive(ctx context.Context) ([]*model.GatewayService, error)
	FindAllWithSource(ctx context.Context, params url.Values) ([]*model.GatewayServiceWithSource, int64, error)
	FindByID(ctx context.Context, id bson.ObjectID) (*model.GatewayService, error)
	FindByIDWithSource(ctx context.Context, id bson.ObjectID) (*model.GatewayServiceWithSource, error)
	FindByBasePath(ctx context.Context, basePath string) (*model.GatewayService, error)
	CountBySourceAlias(ctx context.Context, sourceAlias string) (int64, error)
	Insert(ctx context.Context, doc *model.GatewayService) error
	Update(ctx context.Context, id bson.ObjectID, fields bson.M) error
	SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error
	HardDelete(ctx context.Context, id bson.ObjectID) error
}

type gatewaySourceAliasChecker interface {
	FindByAlias(ctx context.Context, alias string) (*model.GatewaySource, error)
	Insert(ctx context.Context, doc *model.GatewaySource) error
	Update(ctx context.Context, id bson.ObjectID, fields bson.M) error
	SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error
}

type gatewayServiceSourceCounter interface {
	CountBySourceAlias(ctx context.Context, sourceAlias string) (int64, error)
}

type GatewayServiceService struct {
	repo       gatewayServiceRepo
	sourceRepo gatewaySourceAliasChecker
	selfRepo   gatewayServiceSourceCounter
	redisSync  *GatewayRedisSync
}

func NewGatewayServiceService(repo gatewayServiceRepo, sourceRepo gatewaySourceAliasChecker, redisSync *GatewayRedisSync) *GatewayServiceService {
	return &GatewayServiceService{repo: repo, sourceRepo: sourceRepo, selfRepo: repo, redisSync: redisSync}
}

func (s *GatewayServiceService) List(ctx context.Context, params url.Values) ([]*model.GatewayService, int64, error) {
	return s.repo.FindAll(ctx, params)
}

// ListAllActive returns every non-deleted service with no pagination cap — used for boot-time
// Redis resync where a partial page would silently leave some services unsynced.
func (s *GatewayServiceService) ListAllActive(ctx context.Context) ([]*model.GatewayService, error) {
	return s.repo.FindAllActive(ctx)
}

func (s *GatewayServiceService) ListWithSource(ctx context.Context, params url.Values) ([]*model.GatewayServiceWithSource, int64, error) {
	return s.repo.FindAllWithSource(ctx, params)
}

func (s *GatewayServiceService) GetByID(ctx context.Context, id string) (*model.GatewayService, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, ErrGatewayServiceNotFound
	}
	svc, err := s.repo.FindByID(ctx, oid)
	if err != nil {
		return nil, ErrGatewayServiceNotFound
	}
	return svc, nil
}

func (s *GatewayServiceService) GetByIDWithSource(ctx context.Context, id string) (*model.GatewayServiceWithSource, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, ErrGatewayServiceNotFound
	}
	svc, err := s.repo.FindByIDWithSource(ctx, oid)
	if err != nil {
		return nil, ErrGatewayServiceNotFound
	}
	return svc, nil
}

// stripSourceInputs nullifies the Source field after processing so it is not serialised in the response.
func stripSourceInputs(rps []*model.ResourcePath) {
	for _, rp := range rps {
		rp.Source = nil
	}
}

func genSourceAlias() string {
	return "src-" + bson.NewObjectID().Hex()
}

func validateWildcardPaths(resourcePaths []*model.ResourcePath) []model.InvalidResourcePath {
	var invalid []model.InvalidResourcePath
	for i, rp := range resourcePaths {
		if strings.Contains(rp.Path, "*") && !strings.HasSuffix(rp.Path, "/*") {
			invalid = append(invalid, model.InvalidResourcePath{Index: i, Path: rp.Path, SourceAlias: rp.SourceAlias})
		}
	}
	return invalid
}

func (s *GatewayServiceService) prepareResourcePathSources(ctx context.Context, resourcePaths []*model.ResourcePath, ctxUser *model.User) (toInsert, toUpdate []*model.GatewaySource, exc *model.Exception) {
	if invalidPaths := validateWildcardPaths(resourcePaths); len(invalidPaths) > 0 {
		return nil, nil, buildResourcePathException(tool.CodeBodyInvalidFormat, 400, "Wildcard '*' must appear only as the last segment (e.g. /path/*)", "path", "wildcard '*' must appear only as the last segment", invalidPaths)
	}

	now := tool.NowUTC()
	var dupAliases, invalidAliases []model.InvalidResourcePath
	processedAliases := make(map[string]struct{})

	for i, rp := range resourcePaths {
		if rp.Source != nil {
			if rp.SourceAlias != "" {
				if _, seen := processedAliases[rp.SourceAlias]; seen {
					rp.Source = nil
					continue
				}
			}
			src, isUpdate, dup, err := s.resolveInlineSource(ctx, i, rp, &now, ctxUser)
			if err != nil {
				return nil, nil, &model.Exception{Code: tool.CodeOperationFailed, Status: 422, Detail: err.Error()}
			}
			if rp.SourceAlias != "" {
				processedAliases[rp.SourceAlias] = struct{}{}
			}
			if dup != nil {
				dupAliases = append(dupAliases, *dup)
			} else if src != nil {
				if isUpdate {
					toUpdate = append(toUpdate, src)
				} else {
					toInsert = append(toInsert, src)
				}
			}
		} else {
			if inv := s.validateAliasExists(ctx, i, rp); inv != nil {
				invalidAliases = append(invalidAliases, *inv)
			}
		}
	}

	if len(dupAliases) > 0 {
		return nil, nil, buildResourcePathException(tool.CodeGatewaySourceAliasDuplicate, 400, "One or more 'sourceAlias' values already exist", "sourceAlias", "alias already exists", dupAliases)
	}
	if len(invalidAliases) > 0 {
		return nil, nil, buildResourcePathException(tool.CodeGatewayServiceSourceAliasInvalid, 400, "One or more 'sourceAlias' values do not exist", "sourceAlias", "alias does not exist", invalidAliases)
	}
	return toInsert, toUpdate, nil
}

// resolveInlineSource returns (source, isUpdate, invalidResourcePath, error).
// isUpdate=true means the source already existed and was updated in-place.
func (s *GatewayServiceService) resolveInlineSource(ctx context.Context, i int, rp *model.ResourcePath, now *time.Time, ctxUser *model.User) (*model.GatewaySource, bool, *model.InvalidResourcePath, error) {
	if rp.SourceAlias == "" {
		rp.SourceAlias = genSourceAlias()
	}
	if existing, err := s.sourceRepo.FindByAlias(ctx, rp.SourceAlias); err == nil && existing != nil {
		// alias already exists — update it with new inline source data then return it for redis sync
		name := rp.Source.Name
		if name == "" {
			name = existing.Name
		}
		fields := bson.M{
			"name":        name,
			"description": rp.Source.Description,
			"type":        rp.Source.Type,
			"protocol":    rp.Source.Protocol,
			"url":         rp.Source.URL,
			"headers":     rp.Source.Headers,
			"contentType": rp.Source.ContentType,
			"body":        rp.Source.Body,
			"updatedAt":   *now,
			"updatedBy":   ctxUser.ID,
		}
		if err := s.sourceRepo.Update(ctx, existing.ID, fields); err != nil {
			return nil, false, nil, err
		}
		existing.Name = name
		existing.Type = rp.Source.Type
		existing.Protocol = rp.Source.Protocol
		existing.URL = rp.Source.URL
		existing.Headers = rp.Source.Headers
		existing.ContentType = rp.Source.ContentType
		existing.Body = rp.Source.Body
		existing.UpdatedAt = now
		existing.UpdatedBy = &ctxUser.ID
		rp.Source = nil
		return existing, true, nil, nil
	}
	name := rp.Source.Name
	if name == "" {
		name = rp.SourceAlias
	}
	return &model.GatewaySource{
		ID:          bson.NewObjectID(),
		Alias:       rp.SourceAlias,
		Name:        name,
		Description: rp.Source.Description,
		Type:        rp.Source.Type,
		Protocol:    rp.Source.Protocol,
		URL:         rp.Source.URL,
		Headers:     rp.Source.Headers,
		ContentType: rp.Source.ContentType,
		Body:        rp.Source.Body,
		CreatedAt:   now,
		CreatedBy:   &ctxUser.ID,
		UpdatedAt:   now,
		UpdatedBy:   &ctxUser.ID,
	}, false, nil, nil
}

func (s *GatewayServiceService) validateAliasExists(ctx context.Context, i int, rp *model.ResourcePath) *model.InvalidResourcePath {
	if rp.SourceAlias == "" {
		return nil
	}
	if _, err := s.sourceRepo.FindByAlias(ctx, rp.SourceAlias); err != nil {
		return &model.InvalidResourcePath{Index: i, Path: rp.Path, SourceAlias: rp.SourceAlias}
	}
	return nil
}

// สร้าง Exception สำหรับปัญหาเกี่ยวกับ resource paths โดยรวมข้อมูลที่เกี่ยวข้องกับแต่ละ path ที่มีปัญหาไว้ใน properties.fields และ properties.invalidResourcePaths
func buildResourcePathException(code string, status int, detail, fieldName, fieldMsg string, items []model.InvalidResourcePath) *model.Exception {
	fields := make(map[string]interface{}, len(items))
	for _, item := range items {
		key := fmt.Sprintf("resourcePaths[%d].%s", item.Index, fieldName)
		fields[key] = map[string]string{"code": code, "message": fieldMsg}
	}
	invalidList := make([]interface{}, len(items))
	for i, item := range items {
		invalidList[i] = item
	}
	return &model.Exception{
		Code:   code,
		Status: status,
		Detail: detail,
		Properties: map[string]interface{}{
			"fields":               fields,
			"invalidResourcePaths": invalidList,
		},
	}
}

func derefBool(p *bool, def bool) bool {
	if p != nil {
		return *p
	}
	return def
}

// toServiceWithSource converts a GatewayService to GatewayServiceWithSource by looking up sources from a pre-built map.
func toServiceWithSource(svc *model.GatewayService, sourceMap map[string]*model.GatewaySource) *model.GatewayServiceWithSource {
	rps := make([]*model.ResourcePathWithSource, 0, len(svc.ResourcePaths))
	for _, rp := range svc.ResourcePaths {
		entry := &model.ResourcePathWithSource{
			Path:        rp.Path,
			Methods:     rp.Methods,
			SourceAlias: rp.SourceAlias,
		}
		if src, ok := sourceMap[rp.SourceAlias]; ok {
			entry.Source = src
		}
		rps = append(rps, entry)
	}
	return &model.GatewayServiceWithSource{
		ID:            svc.ID,
		Name:          svc.Name,
		Description:   svc.Description,
		Type:          svc.Type,
		BasePath:      svc.BasePath,
		Enabled:       svc.Enabled,
		IsPublic:      svc.IsPublic,
		ResourcePaths: rps,
		CreatedAt:     svc.CreatedAt,
		CreatedBy:     svc.CreatedBy,
		UpdatedAt:     svc.UpdatedAt,
		UpdatedBy:     svc.UpdatedBy,
	}
}

func (s *GatewayServiceService) Create(ctx context.Context, body *model.GatewayServiceCreate, ctxUser *model.User) (*model.GatewayServiceWithSource, error) {
	if existing, err := s.repo.FindByBasePath(ctx, body.BasePath); err == nil && existing != nil {
		return nil, ErrGatewayServiceBasePathDuplicate
	}

	var sourcesToInsert []*model.GatewaySource
	if len(body.ResourcePaths) > 0 {
		srcs, _, exc := s.prepareResourcePathSources(ctx, body.ResourcePaths, ctxUser)
		if exc != nil {
			return nil, exc
		}
		sourcesToInsert = srcs
		if conflicts := checkInternalPathConflicts(extractPaths(body.ResourcePaths)); len(conflicts) > 0 {
			return nil, ErrGatewayServiceResourcePathConflict
		}
	}

	stripSourceInputs(body.ResourcePaths)

	now := tool.NowUTC()
	enabled := derefBool(body.Enabled, true)
	isPublic := derefBool(body.IsPublic, false)

	doc := &model.GatewayService{
		ID:            bson.NewObjectID(),
		Name:          body.Name,
		Description:   body.Description,
		Type:          body.Type,
		BasePath:      body.BasePath,
		Enabled:       &enabled,
		IsPublic:      &isPublic,
		ResourcePaths: body.ResourcePaths,
		CreatedAt:     &now,
		CreatedBy:     &ctxUser.ID,
		UpdatedAt:     &now,
		UpdatedBy:     &ctxUser.ID,
	}

	if err := s.repo.Insert(ctx, doc); err != nil {
		return nil, err
	}

	for _, src := range sourcesToInsert {
		if err := s.sourceRepo.Insert(ctx, src); err != nil {
			// rollback: remove the service we just created
			_ = s.repo.HardDelete(ctx, doc.ID)
			return nil, err
		}
		_ = s.redisSync.SyncSource(ctx, src)
	}

	_ = s.redisSync.SyncService(ctx, doc)

	sourceMap := make(map[string]*model.GatewaySource, len(sourcesToInsert))
	for _, src := range sourcesToInsert {
		sourceMap[src.Alias] = src
	}
	return toServiceWithSource(doc, sourceMap), nil
}

func (s *GatewayServiceService) Update(ctx context.Context, id string, body *model.GatewayServiceUpdate, ctxUser *model.User) (*model.GatewayServiceWithSource, error) {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, ErrGatewayServiceNotFound
	}
	existing, err := s.repo.FindByID(ctx, oid)
	if err != nil {
		return nil, ErrGatewayServiceNotFound
	}
	if body.BasePath != existing.BasePath {
		if dup, err := s.repo.FindByBasePath(ctx, body.BasePath); err == nil && dup != nil {
			return nil, ErrGatewayServiceBasePathDuplicate
		}
	}

	var sourcesToInsert, sourcesToUpdate []*model.GatewaySource
	if len(body.ResourcePaths) > 0 {
		srcs, upd, exc := s.prepareResourcePathSources(ctx, body.ResourcePaths, ctxUser)
		if exc != nil {
			return nil, exc
		}
		sourcesToInsert = srcs
		sourcesToUpdate = upd
		if conflicts := checkInternalPathConflicts(extractPaths(body.ResourcePaths)); len(conflicts) > 0 {
			return nil, ErrGatewayServiceResourcePathConflict
		}
	}

	_, removedAliases := s.diffSources(existing.ResourcePaths, body.ResourcePaths)
	stripSourceInputs(body.ResourcePaths)

	enabled := derefBool(body.Enabled, true)
	isPublic := derefBool(body.IsPublic, false)
	now := tool.NowUTC()
	fields := bson.M{
		"updatedAt":     now,
		"updatedBy":     ctxUser.ID,
		"name":          body.Name,
		"description":   body.Description,
		"type":          body.Type,
		"basePath":      body.BasePath,
		"enabled":       enabled,
		"isPublic":      isPublic,
		"resourcePaths": body.ResourcePaths,
	}

	if err := s.repo.Update(ctx, existing.ID, fields); err != nil {
		return nil, err
	}

	if err := s.insertNewSources(ctx, sourcesToInsert); err != nil {
		_ = s.repo.Update(ctx, existing.ID, bson.M{
			"updatedAt":     existing.UpdatedAt,
			"updatedBy":     existing.UpdatedBy,
			"name":          existing.Name,
			"description":   existing.Description,
			"type":          existing.Type,
			"basePath":      existing.BasePath,
			"enabled":       existing.Enabled,
			"isPublic":      existing.IsPublic,
			"resourcePaths": existing.ResourcePaths,
		})
		return nil, err
	}
	s.syncUpdatedSources(ctx, sourcesToUpdate)
	s.deleteOrphanedSources(ctx, removedAliases, now, ctxUser.ID)

	plain, err := s.repo.FindByID(ctx, existing.ID)
	if err != nil {
		return nil, err
	}
	_ = s.redisSync.SyncService(ctx, plain)

	withSource, err := s.repo.FindByIDWithSource(ctx, existing.ID)
	if err != nil {
		return nil, err
	}
	return withSource, nil
}

// diffSources returns sources to insert (new inline sources) and aliases removed from resourcePaths.
// Note: prepareResourcePathSources mutates rp.SourceAlias for auto-gen, so it must be called before this.
func (s *GatewayServiceService) diffSources(oldRPs, newRPs []*model.ResourcePath) (toInsert []*model.GatewaySource, removedAliases []string) {
	newAliasSet := make(map[string]struct{}, len(newRPs))
	for _, rp := range newRPs {
		if rp.SourceAlias != "" {
			newAliasSet[rp.SourceAlias] = struct{}{}
		}
	}
	for _, rp := range oldRPs {
		if rp.SourceAlias == "" {
			continue
		}
		if _, kept := newAliasSet[rp.SourceAlias]; !kept {
			removedAliases = append(removedAliases, rp.SourceAlias)
		}
	}
	return
}

func (s *GatewayServiceService) insertNewSources(ctx context.Context, sources []*model.GatewaySource) error {
	for _, src := range sources {
		if err := s.sourceRepo.Insert(ctx, src); err != nil {
			return err
		}
		_ = s.redisSync.SyncSource(ctx, src)
	}
	return nil
}

func (s *GatewayServiceService) syncUpdatedSources(ctx context.Context, sources []*model.GatewaySource) {
	for _, src := range sources {
		_ = s.redisSync.SyncSource(ctx, src)
	}
}

func (s *GatewayServiceService) deleteOrphanedSources(ctx context.Context, aliases []string, now time.Time, deletedBy bson.ObjectID) {
	for _, alias := range aliases {
		src, err := s.sourceRepo.FindByAlias(ctx, alias)
		if err != nil {
			continue
		}
		count, err := s.selfRepo.CountBySourceAlias(ctx, alias)
		if err != nil || count > 0 {
			continue
		}
		if err := s.sourceRepo.SoftDelete(ctx, src.ID, now, deletedBy); err == nil {
			_ = s.redisSync.DeleteSource(ctx, alias)
		}
	}
}

func (s *GatewayServiceService) Delete(ctx context.Context, id string, forever bool, ctxUser *model.User) error {
	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return ErrGatewayServiceNotFound
	}

	existing, err := s.repo.FindByID(ctx, oid)
	if err != nil {
		return ErrGatewayServiceNotFound
	}

	now := tool.NowUTC()

	// soft delete sources that are no longer referenced after this service is removed
	s.cleanupOrphanedSources(ctx, existing.ResourcePaths, now, ctxUser.ID)

	if forever {
		if err := s.repo.HardDelete(ctx, existing.ID); err != nil {
			return err
		}
		_ = s.redisSync.DeleteService(ctx, existing.ID.Hex())
		return nil
	}

	if err := s.repo.SoftDelete(ctx, existing.ID, now, ctxUser.ID); err != nil {
		return err
	}
	_ = s.redisSync.DeleteService(ctx, existing.ID.Hex())
	return nil
}

func (s *GatewayServiceService) cleanupOrphanedSources(ctx context.Context, resourcePaths []*model.ResourcePath, now time.Time, deletedBy bson.ObjectID) {
	for _, rp := range resourcePaths {
		if rp.SourceAlias == "" {
			continue
		}
		src, err := s.sourceRepo.FindByAlias(ctx, rp.SourceAlias)
		if err != nil {
			continue
		}
		count, err := s.selfRepo.CountBySourceAlias(ctx, rp.SourceAlias)
		if err != nil || count > 1 {
			continue
		}
		if err := s.sourceRepo.SoftDelete(ctx, src.ID, now, deletedBy); err == nil {
			_ = s.redisSync.DeleteSource(ctx, rp.SourceAlias)
		}
	}
}

func (s *GatewayServiceService) FindSourcesByAliases(ctx context.Context, aliases []string) map[string]*model.GatewaySource {
	result := make(map[string]*model.GatewaySource)
	for _, alias := range aliases {
		if alias == "" {
			continue
		}
		if _, ok := result[alias]; ok {
			continue
		}
		if src, err := s.sourceRepo.FindByAlias(ctx, alias); err == nil {
			result[alias] = src
		}
	}
	return result
}

func (s *GatewayServiceService) CheckPaths(ctx context.Context, basePath string, resourcePaths []string, excludeID *bson.ObjectID) (*model.GatewayServiceCheckPathsResponse, error) {
	resp := &model.GatewayServiceCheckPathsResponse{}

	if existing, err := s.repo.FindByBasePath(ctx, basePath); err == nil && existing != nil {
		if excludeID == nil || existing.ID != *excludeID {
			resp.HasConflict = true
			resp.BasePathConflict = &model.BasePathConflictInfo{
				ConflictingServiceID:   existing.ID.Hex(),
				ConflictingServiceName: existing.Name,
				ConflictingBasePath:    existing.BasePath,
			}
		}
	}

	if conflicts := checkInternalPathConflicts(resourcePaths); len(conflicts) > 0 {
		resp.HasConflict = true
		resp.ResourcePathConflicts = conflicts
	}

	return resp, nil
}

// checkInternalPathConflicts groups paths by normalized pattern and returns each group with more than one path
func checkInternalPathConflicts(paths []string) []*model.InternalPathConflict {
	groups := make(map[string][]string)
	order := []string{}
	for _, p := range paths {
		norm := normalizePath(p)
		if _, seen := groups[norm]; !seen {
			order = append(order, norm)
		}
		groups[norm] = append(groups[norm], p)
	}
	var conflicts []*model.InternalPathConflict
	for _, norm := range order {
		if len(groups[norm]) > 1 {
			conflicts = append(conflicts, &model.InternalPathConflict{
				ConflictingPaths: groups[norm],
			})
		}
	}
	return conflicts
}

// extractPaths ดึงเฉพาะ path string จาก ResourcePath slice
func extractPaths(rps []*model.ResourcePath) []string {
	paths := make([]string, len(rps))
	for i, rp := range rps {
		paths[i] = rp.Path
	}
	return paths
}

// normalizePath แทน {param} ด้วย {*} เพื่อใช้เปรียบเทียบ pattern
func normalizePath(p string) string {
	segments := strings.Split(p, "/")
	for i, seg := range segments {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			segments[i] = "{*}"
		}
	}
	return strings.Join(segments, "/")
}

// pathsConflict คืน true ถ้า path ทั้งสองมี normalized pattern เหมือนกัน
// static > param > wildcard ใน routing priority จึงตรวจแค่ exact pattern match
func pathsConflict(a, b string) bool {
	return normalizePath(a) == normalizePath(b)
}

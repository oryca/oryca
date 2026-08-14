package service

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/oryca/oryca/control-plane/logger"
	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/tool"

	"go.mongodb.org/mongo-driver/v2/bson"
)

var (
	ErrApiKeyNotFound  = errors.New("api key not found")
	ErrApiKeyForbidden = errors.New("forbidden")
)

type apiKeyRepo interface {
	FindAll(ctx context.Context, params url.Values, ownerIDs []bson.ObjectID, excludeOwnerIDs []bson.ObjectID) ([]*model.ApiKey, int64, error)
	FindByID(ctx context.Context, id bson.ObjectID) (*model.ApiKey, error)
	FindByKey(ctx context.Context, key string) (*model.ApiKey, error)
	Insert(ctx context.Context, doc *model.ApiKey) error
	Update(ctx context.Context, id bson.ObjectID, fields bson.M) error
	SoftDelete(ctx context.Context, id bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error
	HardDelete(ctx context.Context, id bson.ObjectID) error
	BulkSoftDelete(ctx context.Context, ids []bson.ObjectID, deletedAt time.Time, deletedBy bson.ObjectID) error
	BulkHardDelete(ctx context.Context, ids []bson.ObjectID) error
}

type apiKeyCache interface {
	Get(ctx context.Context, keyValue string) (*model.ApiKey, error)
	Set(ctx context.Context, ak *model.ApiKey, owner *model.User) error
	Delete(ctx context.Context, keyValues []string) error
}

type apiKeyUserFinder interface {
	FindByID(ctx context.Context, id bson.ObjectID) (*model.User, error)
}

type apiKeyNotifier interface {
	Create(ctx context.Context, in *model.NotificationCreate) error
}

// apiKeyGatewayPublisher is the narrow slice of GatewayEventPublisher this service
// needs — real-time sync event push to the gateway fanout exchange.
type apiKeyGatewayPublisher interface {
	Publish(eventType string, payload any)
}

type ApiKeyService struct {
	repo      apiKeyRepo
	cache     apiKeyCache
	userRepo  apiKeyUserFinder
	notifier  apiKeyNotifier
	publisher apiKeyGatewayPublisher
}

func NewApiKeyService(repo apiKeyRepo, cache apiKeyCache, userRepo apiKeyUserFinder, notifier apiKeyNotifier, publisher apiKeyGatewayPublisher) *ApiKeyService {
	return &ApiKeyService{repo: repo, cache: cache, userRepo: userRepo, notifier: notifier, publisher: publisher}
}

// publishApiKeyEvent sends the gateway-facing shape of ak (same as GetApiKeys' internal
// endpoint payload, minus batching) as a full-payload real-time sync event. Best-effort:
// GatewayEventPublisher.Publish never blocks or returns an error — HTTP polling on the
// gateway side still catches this within its own interval either way.
func (s *ApiKeyService) publishApiKeyEvent(ak *model.ApiKey, owner *model.User) {
	if s.publisher == nil {
		return
	}
	payload := &model.ApiKeyCache{
		ID:        ak.ID.Hex(),
		ApiKey:    ak.ApiKey,
		Enabled:   ak.Enabled,
		ExpiredAt: ak.ExpiredAt,
	}
	if owner != nil {
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
	s.publisher.Publish(SyncEventTypeApiKey, payload)
}

// wasEnabled คืน true ถ้า enabled เดิมถือว่าเปิดใช้งานอยู่ (nil = default enabled)
func wasEnabled(enabled *bool) bool {
	return enabled == nil || *enabled
}

// canManageForUser คืน true ถ้า actor สามารถสร้าง/จัดการ key ให้ target role ได้
// root → ได้ทุก role | admin → user
func (s *ApiKeyService) canManageForUser(ctx context.Context, actor *model.User, targetRole string) bool {
	switch actor.Role {
	case "root":
		return true
	case "admin":
		return targetRole == "user"
	default:
		return false
	}
}

// canViewOtherKeys คืน true ถ้า actor สามารถดู key ของ user อื่นได้
func (s *ApiKeyService) canViewOtherKeys(ctx context.Context, actor *model.User) bool {
	if actor.Role == "root" || actor.Role == "admin" {
		return true
	}
	return false
}

// isOwner คืน true ถ้า actor เป็นเจ้าของ key
func isOwner(actor *model.User, ak *model.ApiKey) bool {
	return ak.OwnerBy != nil && *ak.OwnerBy == actor.ID
}

// canModifyKey คืน true ถ้า actor สามารถแก้ไข key นี้ได้
// root/admin → ทุก key | user → เฉพาะ ownerBy ตัวเอง
func (s *ApiKeyService) canModifyKey(actor *model.User, ak *model.ApiKey) bool {
	if actor.Role == "root" || actor.Role == "admin" {
		return true
	}
	return isOwner(actor, ak)
}

func (s *ApiKeyService) List(ctx context.Context, params url.Values, ctxUser *model.User) ([]*model.ApiKey, int64, error) {
	ownerIDs, excludeOwnerIDs, err := s.resolveOwnerFilter(ctx, params["ownerBy"], ctxUser)
	if err != nil {
		return nil, 0, err
	}
	return s.repo.FindAll(ctx, params, ownerIDs, excludeOwnerIDs)
}

// resolveOwnerFilter แปลง ownerBy query params เป็น ownerIDs และ excludeOwnerIDs
func (s *ApiKeyService) resolveOwnerFilter(ctx context.Context, rawValues []string, ctxUser *model.User) (ownerIDs, excludeOwnerIDs []bson.ObjectID, err error) {
	includeRaw, excludeRaw := parseOwnerByValues(rawValues)
	hasFilter := len(includeRaw) > 0 || len(excludeRaw) > 0

	if !hasFilter {
		// ไม่ระบุ ownerBy — root/admin ดูทั้งหมด, user ดูเฉพาะของตัวเอง
		if !s.canViewOtherKeys(ctx, ctxUser) {
			ownerIDs = []bson.ObjectID{ctxUser.ID}
		}
		return ownerIDs, nil, nil
	}

	if !s.canViewOtherKeys(ctx, ctxUser) {
		return nil, nil, ErrApiKeyForbidden
	}

	for _, raw := range includeRaw {
		id, err := bson.ObjectIDFromHex(raw)
		if err != nil {
			return nil, nil, err
		}
		ownerIDs = append(ownerIDs, id)
	}
	for _, raw := range excludeRaw {
		id, err := bson.ObjectIDFromHex(raw)
		if err != nil {
			return nil, nil, err
		}
		excludeOwnerIDs = append(excludeOwnerIDs, id)
	}
	return ownerIDs, excludeOwnerIDs, nil
}

// parseOwnerByValues แยก include (id) และ exclude (!id) จาก raw query values
func parseOwnerByValues(rawValues []string) (include, exclude []string) {
	for _, v := range rawValues {
		for _, part := range strings.Split(v, ",") {
			part = strings.TrimSpace(part)
			switch {
			case strings.HasPrefix(part, "!"):
				exclude = append(exclude, part[1:])
			case part != "":
				include = append(include, part)
			}
		}
	}
	return include, exclude
}

func (s *ApiKeyService) GetByID(ctx context.Context, id bson.ObjectID, ctxUser *model.User) (*model.ApiKey, error) {
	ak, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, ErrApiKeyNotFound
	}

	if !isOwner(ctxUser, ak) && !s.canViewOtherKeys(ctx, ctxUser) {
		return nil, ErrApiKeyForbidden
	}

	return ak, nil
}

func (s *ApiKeyService) GetByKey(ctx context.Context, keyValue string) (*model.ApiKey, error) {
	cached, err := s.cache.Get(ctx, keyValue)
	if err == nil && cached != nil {
		return cached, nil
	}

	ak, err := s.repo.FindByKey(ctx, keyValue)
	if err != nil {
		return nil, ErrApiKeyNotFound
	}

	var owner *model.User
	if ak.OwnerBy != nil {
		owner, _ = s.userRepo.FindByID(ctx, *ak.OwnerBy)
	}
	_ = s.cache.Set(ctx, ak, owner)
	return ak, nil
}

func (s *ApiKeyService) Create(ctx context.Context, body *model.ApiKeyCreate, ctxUser *model.User, targetUser *model.User) (*model.ApiKey, error) {
	ownerID := ctxUser.ID

	if targetUser != nil && targetUser.ID != ctxUser.ID {
		if !s.canManageForUser(ctx, ctxUser, targetUser.Role) {
			return nil, ErrApiKeyForbidden
		}
		ownerID = targetUser.ID
	}

	keyValue, err := tool.GenerateApiKey()
	if err != nil {
		return nil, err
	}

	now := tool.NowUTC()
	t := true
	enabled := &t
	if body.Enabled != nil {
		enabled = body.Enabled
	}

	createdBy := ctxUser.ID
	doc := &model.ApiKey{
		ID:          bson.NewObjectID(),
		Name:        body.Name,
		Description: body.Description,
		ApiKey:      keyValue,
		Enabled:     enabled,
		ExpiredAt:   body.ExpiredAt,
		OwnerBy:     &ownerID,
		CreatedAt:   &now,
		CreatedBy:   &createdBy,
		UpdatedAt:   &now,
		UpdatedBy:   &createdBy,
	}

	if err := s.repo.Insert(ctx, doc); err != nil {
		return nil, err
	}

	owner := ctxUser
	if targetUser != nil {
		owner = targetUser
	}
	_ = s.cache.Set(ctx, doc, owner)
	s.publishApiKeyEvent(doc, owner)
	return doc, nil
}

func (s *ApiKeyService) Update(ctx context.Context, id bson.ObjectID, body *model.ApiKeyUpdate, ctxUser *model.User) (*model.ApiKey, error) {
	ak, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, ErrApiKeyNotFound
	}

	if !s.canModifyKey(ctxUser, ak) {
		return nil, ErrApiKeyForbidden
	}

	now := tool.NowUTC()
	fields := bson.M{
		"name":        body.Name,
		"description": body.Description,
		"enabled":     body.Enabled,
		"expiredAt":   body.ExpiredAt,
		"updatedAt":   now,
		"updatedBy":   ctxUser.ID,
	}

	wasSuspended := wasEnabled(ak.Enabled) && body.Enabled != nil && !*body.Enabled

	if err := s.repo.Update(ctx, id, fields); err != nil {
		return nil, err
	}

	// อัปเดต cache เดิม (ลบออก — จะถูก set ใหม่เมื่อ GetByKey ถูกเรียก)
	_ = s.cache.Delete(ctx, []string{ak.ApiKey})

	updated, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	var owner *model.User
	if updated.OwnerBy != nil {
		owner, _ = s.userRepo.FindByID(ctx, *updated.OwnerBy)
	}
	_ = s.cache.Set(ctx, updated, owner)
	s.publishApiKeyEvent(updated, owner)

	// Commit -> Notify: แจ้งเตือนหลัง commit สำเร็จเท่านั้น
	if wasSuspended && updated.OwnerBy != nil && s.notifier != nil {
		runNotifyBackground(ctx, 30*time.Second, func(bgCtx context.Context) {
			s.notifyApiKeySuspended(bgCtx, updated, body.Reason, ctxUser.ID)
		})
	}

	return updated, nil
}

func (s *ApiKeyService) notifyApiKeySuspended(ctx context.Context, ak *model.ApiKey, reason *string, actorID bson.ObjectID) {
	detail := map[string]any{
		"apiKeyId": ak.ID.Hex(),
		"keyAlias": ak.Name,
	}
	if reason != nil && *reason != "" {
		detail["reason"] = *reason
	}

	if err := s.notifier.Create(ctx, &model.NotificationCreate{
		UserID:    *ak.OwnerBy,
		Topic:     model.NotificationTopicApiKeySuspended,
		Level:     model.NotificationLevelWarning,
		Title:     "API Key Suspended",
		Message:   "Your API key \"" + ak.Name + "\" has been suspended",
		DedupKey:  "apikey_suspended:" + ak.ID.Hex(),
		Detail:    detail,
		CreatedBy: actorID,
	}); err != nil {
		logger.Error("apikey_suspended notification failed: " + err.Error())
	}
}

func (s *ApiKeyService) Regenerate(ctx context.Context, id bson.ObjectID, ctxUser *model.User) (*model.ApiKey, error) {
	ak, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, ErrApiKeyNotFound
	}

	if !s.canModifyKey(ctxUser, ak) {
		return nil, ErrApiKeyForbidden
	}

	newKey, err := tool.GenerateApiKey()
	if err != nil {
		return nil, err
	}

	oldKey := ak.ApiKey
	now := tool.NowUTC()
	fields := bson.M{
		"apiKey":    newKey,
		"updatedAt": now,
		"updatedBy": ctxUser.ID,
	}

	if err := s.repo.Update(ctx, id, fields); err != nil {
		return nil, err
	}

	_ = s.cache.Delete(ctx, []string{oldKey})

	updated, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	var owner *model.User
	if updated.OwnerBy != nil {
		owner, _ = s.userRepo.FindByID(ctx, *updated.OwnerBy)
	}
	_ = s.cache.Set(ctx, updated, owner)
	s.publishApiKeyEvent(updated, owner)
	return updated, nil
}

func (s *ApiKeyService) Delete(ctx context.Context, id bson.ObjectID, forever bool, ctxUser *model.User) error {
	ak, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return ErrApiKeyNotFound
	}

	if !s.canModifyKey(ctxUser, ak) {
		return ErrApiKeyForbidden
	}

	_ = s.cache.Delete(ctx, []string{ak.ApiKey})

	if forever {
		return s.repo.HardDelete(ctx, id)
	}

	now := tool.NowUTC()
	return s.repo.SoftDelete(ctx, id, now, ctxUser.ID)
}

func (s *ApiKeyService) BulkDelete(ctx context.Context, body *model.ApiKeyBulkDelete, ctxUser *model.User) error {
	var keyValues []string

	for _, id := range body.ApiKeyIDs {
		ak, err := s.repo.FindByID(ctx, id)
		if err != nil {
			continue // ถ้าไม่เจอก็ข้ามไป
		}
		if !s.canModifyKey(ctxUser, ak) {
			return ErrApiKeyForbidden
		}
		keyValues = append(keyValues, ak.ApiKey)
	}

	_ = s.cache.Delete(ctx, keyValues)

	if body.Forever {
		return s.repo.BulkHardDelete(ctx, body.ApiKeyIDs)
	}

	now := tool.NowUTC()
	return s.repo.BulkSoftDelete(ctx, body.ApiKeyIDs, now, ctxUser.ID)
}

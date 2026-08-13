package service

import (
	"context"
	"net/url"
	"time"

	"github.com/oryca/oryca/control-plane/logger"
	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/tool"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type packageUserRepo interface {
	FindByPackageID(ctx context.Context, packageID bson.ObjectID, params url.Values) ([]*model.User, int64, error)
	BulkSetPackageID(ctx context.Context, userIDs []bson.ObjectID, packageID bson.ObjectID, now time.Time, updatedBy bson.ObjectID) (int64, error)
	BulkUnsetPackageID(ctx context.Context, packageID bson.ObjectID, userIDs []bson.ObjectID, now time.Time, updatedBy bson.ObjectID) (int64, error)
	CountByOldPackageIDs(ctx context.Context, userIDs []bson.ObjectID, newPackageID bson.ObjectID) (map[bson.ObjectID]int64, error)
}

type packageUserCountRepo interface {
	IncrUserCount(ctx context.Context, id bson.ObjectID, delta int64) error
}

type groupDescendantRepo interface {
	FindDescendantGroupIDs(ctx context.Context, groupID bson.ObjectID) ([]bson.ObjectID, error)
}

type groupUserIDsRepo interface {
	FindUserIDsByGroupIDs(ctx context.Context, groupIDs []bson.ObjectID) ([]bson.ObjectID, error)
}

type packageUserNotifier interface {
	Create(ctx context.Context, in *model.NotificationCreate) error
}

// packageUserGatewayPublisher is the narrow slice of GatewayEventPublisher this service needs.
type packageUserGatewayPublisher interface {
	Publish(eventType string, payload any)
}

type PackageUserService struct {
	userRepo      packageUserRepo
	packageRepo   packageFinder
	pkgRepo       packageUserCountRepo
	groupRepo     groupDescendantRepo
	groupUserRepo groupUserIDsRepo
	notifier      packageUserNotifier
	publisher     packageUserGatewayPublisher
}

func NewPackageUserService(userRepo packageUserRepo, packageRepo packageFinder, pkgRepo packageUserCountRepo, groupRepo groupDescendantRepo, groupUserRepo groupUserIDsRepo, notifier packageUserNotifier, publisher packageUserGatewayPublisher) *PackageUserService {
	return &PackageUserService{userRepo: userRepo, packageRepo: packageRepo, pkgRepo: pkgRepo, groupRepo: groupRepo, groupUserRepo: groupUserRepo, notifier: notifier, publisher: publisher}
}

// publishUserInvalidate fires exactly one thin invalidate event per bulk call, carrying
// the affected ids — never one event per user, which would reproduce the event-storm
// bug already fixed for reload_services (a groupID can expand to hundreds of users in
// one BulkSetPackageID/BulkUnsetPackageID call).
func (s *PackageUserService) publishUserInvalidate(ids []bson.ObjectID) {
	if s.publisher == nil {
		return
	}
	hexIDs := make([]string, len(ids))
	for i, id := range ids {
		hexIDs[i] = id.Hex()
	}
	s.publisher.Publish(SyncEventTypeUserInvalidate, &model.UserIDsPayload{UserIDs: hexIDs})
}

func (s *PackageUserService) resolveGroupUserIDs(ctx context.Context, groupID string) ([]string, error) {
	gid, err := bson.ObjectIDFromHex(groupID)
	if err != nil {
		return nil, err
	}
	groupIDs, err := s.groupRepo.FindDescendantGroupIDs(ctx, gid)
	if err != nil {
		return nil, err
	}
	userOIDs, err := s.groupUserRepo.FindUserIDsByGroupIDs(ctx, groupIDs)
	if err != nil {
		return nil, err
	}
	result := make([]string, len(userOIDs))
	for i, id := range userOIDs {
		result[i] = id.Hex()
	}
	return result, nil
}

func (s *PackageUserService) GetUsers(ctx context.Context, packageID string, params url.Values) ([]*model.User, int64, error) {
	pid, err := bson.ObjectIDFromHex(packageID)
	if err != nil {
		return nil, 0, ErrPackageNotFound
	}
	if _, err := s.packageRepo.FindByID(ctx, pid); err != nil {
		return nil, 0, ErrPackageNotFound
	}
	return s.userRepo.FindByPackageID(ctx, pid, params)
}

func (s *PackageUserService) AssignUsers(ctx context.Context, packageID string, rawIDs []string, groupID *string, ctxUser *model.User) (assigned int64, skipped int, err error) {
	pid, err := bson.ObjectIDFromHex(packageID)
	if err != nil {
		return 0, 0, ErrPackageNotFound
	}
	pkg, err := s.packageRepo.FindByID(ctx, pid)
	if err != nil {
		return 0, 0, ErrPackageNotFound
	}

	if groupID != nil {
		groupUserIDs, e := s.resolveGroupUserIDs(ctx, *groupID)
		if e != nil {
			return 0, 0, e
		}
		rawIDs = append(rawIDs, groupUserIDs...)
	}

	seen := make(map[bson.ObjectID]struct{})
	var ids []bson.ObjectID
	for _, raw := range rawIDs {
		id, e := bson.ObjectIDFromHex(raw)
		if e != nil {
			skipped++
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return 0, skipped, nil
	}

	oldCounts, _ := s.userRepo.CountByOldPackageIDs(ctx, ids, pid)

	now := tool.NowUTC()
	assigned, err = s.userRepo.BulkSetPackageID(ctx, ids, pid, now, ctxUser.ID)
	if err == nil && assigned > 0 {
		_ = s.pkgRepo.IncrUserCount(ctx, pid, assigned)
		for oldPkgID, count := range oldCounts {
			_ = s.pkgRepo.IncrUserCount(ctx, oldPkgID, -count)
		}
		s.publishUserInvalidate(ids)

		// ไม่ใส่ oldPackageName เพราะ ids มาจากหลาย package เดิมปนกันได้ (oldCounts เป็น aggregate)
		if s.notifier != nil {
			runNotifyBackground(ctx, 60*time.Second, func(bgCtx context.Context) {
				s.notifyPackageChanged(bgCtx, pkg, ids, ctxUser.ID)
			})
		}
	}
	return assigned, skipped, err
}

func (s *PackageUserService) notifyPackageChanged(ctx context.Context, pkg *model.Package, userIDs []bson.ObjectID, actorID bson.ObjectID) {
	for _, uid := range userIDs {
		if err := s.notifier.Create(ctx, &model.NotificationCreate{
			UserID:  uid,
			Topic:   model.NotificationTopicPackageChanged,
			Level:   model.NotificationLevelInfo,
			Title:   "Package Changed",
			Message: "Your package has been changed to \"" + pkg.Name + "\"",
			Detail: map[string]any{
				"newPackageId":   pkg.ID.Hex(),
				"newPackageName": pkg.Name,
			},
			CreatedBy: actorID,
		}); err != nil {
			logger.Error("package_changed notification failed userId=" + uid.Hex() + " err=" + err.Error())
		}
	}
}

func (s *PackageUserService) RemoveUsers(ctx context.Context, packageID string, rawIDs []string, groupID *string, ctxUser *model.User) (removed int64, skipped int, err error) {
	pid, err := bson.ObjectIDFromHex(packageID)
	if err != nil {
		return 0, 0, ErrPackageNotFound
	}
	if _, err := s.packageRepo.FindByID(ctx, pid); err != nil {
		return 0, 0, ErrPackageNotFound
	}

	if groupID != nil {
		groupUserIDs, e := s.resolveGroupUserIDs(ctx, *groupID)
		if e != nil {
			return 0, 0, e
		}
		rawIDs = append(rawIDs, groupUserIDs...)
	}

	seen := make(map[bson.ObjectID]struct{})
	var ids []bson.ObjectID
	for _, raw := range rawIDs {
		id, e := bson.ObjectIDFromHex(raw)
		if e != nil {
			skipped++
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return 0, skipped, nil
	}

	now := tool.NowUTC()
	removed, err = s.userRepo.BulkUnsetPackageID(ctx, pid, ids, now, ctxUser.ID)
	if err == nil && removed > 0 {
		_ = s.pkgRepo.IncrUserCount(ctx, pid, -removed)
		s.publishUserInvalidate(ids)
	}
	return removed, skipped, err
}

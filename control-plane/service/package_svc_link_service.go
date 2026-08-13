package service

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/oryca/oryca/control-plane/logger"
	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/tool"

	"go.mongodb.org/mongo-driver/v2/bson"
)

var (
	ErrPackageSvcLinkNotFound  = errors.New("package service not found")
	ErrPackageSvcLinkDuplicate = errors.New("service already added to this package")
	ErrPackagePathInvalid      = errors.New("one or more paths or methods are invalid for this service")
)

type packageSvcLinkRepo interface {
	Aggregate(ctx context.Context, packageID *bson.ObjectID, params url.Values) ([]*model.PackageSvcLinkDetail, int64, error)
	FindByPackageAndService(ctx context.Context, packageID, serviceID bson.ObjectID) (*model.PackageSvcLink, error)
	FindPackageIDsByServiceID(ctx context.Context, serviceID bson.ObjectID) ([]string, error)
	Insert(ctx context.Context, doc *model.PackageSvcLink) error
	Update(ctx context.Context, id bson.ObjectID, fields bson.M) error
	Delete(ctx context.Context, packageID, serviceID bson.ObjectID) error
}

type packageGatewayServiceFinder interface {
	FindByID(ctx context.Context, id bson.ObjectID) (*model.GatewayService, error)
}

type packageFinder interface {
	FindByID(ctx context.Context, id bson.ObjectID) (*model.Package, error)
}

type packageIDsSyncer interface {
	SyncServicePackageIDs(ctx context.Context, svc *model.GatewayService, packageIDs []string) error
}

type packageSvcCountRepo interface {
	IncrServiceCount(ctx context.Context, id bson.ObjectID, delta int64) error
}

type packageSvcLinkUserFinder interface {
	FindByPackageID(ctx context.Context, packageID bson.ObjectID, params url.Values) ([]*model.User, int64, error)
}

type packageSvcLinkNotifier interface {
	Create(ctx context.Context, in *model.NotificationCreate) error
}

type PackageSvcLinkService struct {
	repo        packageSvcLinkRepo
	serviceSvc  packageGatewayServiceFinder
	packageRepo packageFinder
	pkgRepo     packageSvcCountRepo
	redisSync   packageIDsSyncer
	userFinder  packageSvcLinkUserFinder
	notifier    packageSvcLinkNotifier
}

func NewPackageSvcLinkService(repo packageSvcLinkRepo, serviceSvc packageGatewayServiceFinder, packageRepo packageFinder, pkgRepo packageSvcCountRepo, redisSync packageIDsSyncer, userFinder packageSvcLinkUserFinder, notifier packageSvcLinkNotifier) *PackageSvcLinkService {
	return &PackageSvcLinkService{repo: repo, serviceSvc: serviceSvc, packageRepo: packageRepo, pkgRepo: pkgRepo, redisSync: redisSync, userFinder: userFinder, notifier: notifier}
}

const packageUserNotifyBatchSize = 500

// notifyServicesUpdated แจ้งเตือนทุกคนในแพ็กเกจ — ดึง user แบบแบ่งหน้าทีละ packageUserNotifyBatchSize
// คุมหน่วยความจำให้คงที่ไม่ว่าแพ็กเกจจะมีสมาชิกกี่คน
func (s *PackageSvcLinkService) notifyServicesUpdated(ctx context.Context, pkg *model.Package, serviceName, action string, actorID bson.ObjectID) {
	if s.notifier == nil || s.userFinder == nil {
		return
	}
	runNotifyBackground(ctx, 5*time.Minute, func(bgCtx context.Context) {
		s.dispatchServicesUpdatedNotifications(bgCtx, pkg, serviceName, action, actorID)
	})
}

func (s *PackageSvcLinkService) dispatchServicesUpdatedNotifications(ctx context.Context, pkg *model.Package, serviceName, action string, actorID bson.ObjectID) {
	actionVerb := map[string]string{"added": "added to", "updated": "updated in", "removed": "removed from"}[action]
	offset := 0
	for {
		params := url.Values{
			"limit":  []string{strconv.Itoa(packageUserNotifyBatchSize)},
			"offset": []string{strconv.Itoa(offset)},
		}
		users, _, err := s.userFinder.FindByPackageID(ctx, pkg.ID, params)
		if err != nil {
			logger.Error("package_services_updated: fetch package users failed: " + err.Error())
			return
		}

		for _, u := range users {
			if err := s.notifier.Create(ctx, &model.NotificationCreate{
				UserID:  u.ID,
				Topic:   model.NotificationTopicPackageServicesUpdated,
				Level:   model.NotificationLevelInfo,
				Title:   "Package Service Updated",
				Message: "Service \"" + serviceName + "\" was " + actionVerb + " package \"" + pkg.Name + "\"",
				Detail: map[string]any{
					"packageName": pkg.Name,
					"serviceName": serviceName,
					"action":      action,
				},
				CreatedBy: actorID,
			}); err != nil {
				logger.Error("package_services_updated notification failed userId=" + u.ID.Hex() + " err=" + err.Error())
			}
		}

		if len(users) < packageUserNotifyBatchSize {
			return
		}
		offset += packageUserNotifyBatchSize
	}
}

func (s *PackageSvcLinkService) syncServicePackages(ctx context.Context, svc *model.GatewayService) {
	ids, err := s.repo.FindPackageIDsByServiceID(ctx, svc.ID)
	if err != nil {
		logger.Error("package_svc_link: syncServicePackages: FindPackageIDsByServiceID failed id=" + svc.ID.Hex() + " err=" + err.Error())
		return
	}
	if err := s.redisSync.SyncServicePackageIDs(ctx, svc, ids); err != nil {
		logger.Error("package_svc_link: syncServicePackages: SyncServicePackageIDs failed id=" + svc.ID.Hex() + " err=" + err.Error())
	}
}

func (s *PackageSvcLinkService) populateDetail(ctx context.Context, link *model.PackageSvcLink) *model.PackageSvcLinkDetail {
	detail := &model.PackageSvcLinkDetail{
		ID:        link.ID,
		Paths:     link.Paths,
		CreatedAt: link.CreatedAt,
		CreatedBy: link.CreatedBy,
		UpdatedAt: link.UpdatedAt,
		UpdatedBy: link.UpdatedBy,
	}
	if svc, err := s.serviceSvc.FindByID(ctx, link.ServiceID); err == nil {
		detail.Service = &model.ServiceSummary{
			ID:          svc.ID,
			Name:        svc.Name,
			Description: svc.Description,
			Type:        svc.Type,
			BasePath:    svc.BasePath,
		}
	}
	if pkg, err := s.packageRepo.FindByID(ctx, link.PackageID); err == nil {
		detail.Package = &model.PackageSummary{
			ID:          pkg.ID,
			Name:        pkg.Name,
			Alias:       pkg.Alias,
			Description: pkg.Description,
		}
	}
	return detail
}

func (s *PackageSvcLinkService) ListByPackage(ctx context.Context, packageID string, params url.Values) ([]*model.PackageSvcLinkDetail, int64, error) {
	pid, err := bson.ObjectIDFromHex(packageID)
	if err != nil {
		return nil, 0, ErrPackageNotFound
	}
	return s.repo.Aggregate(ctx, &pid, params)
}

func (s *PackageSvcLinkService) GetByService(ctx context.Context, packageID, serviceID string) (*model.PackageSvcLinkDetail, error) {
	pid, err := bson.ObjectIDFromHex(packageID)
	if err != nil {
		return nil, ErrPackageSvcLinkNotFound
	}
	sid, err := bson.ObjectIDFromHex(serviceID)
	if err != nil {
		return nil, ErrPackageSvcLinkNotFound
	}

	link, err := s.repo.FindByPackageAndService(ctx, pid, sid)
	if err != nil {
		return nil, ErrPackageSvcLinkNotFound
	}

	return s.populateDetail(ctx, link), nil
}

func (s *PackageSvcLinkService) Create(ctx context.Context, packageID string, body *model.PackageSvcLinkCreate, ctxUser *model.User) (*model.PackageSvcLinkDetail, error) {
	pid, err := bson.ObjectIDFromHex(packageID)
	if err != nil {
		return nil, ErrPackageNotFound
	}
	sid, err := bson.ObjectIDFromHex(body.ServiceID)
	if err != nil {
		return nil, ErrPackageSvcLinkNotFound
	}

	if existing, err := s.repo.FindByPackageAndService(ctx, pid, sid); err == nil && existing != nil {
		return nil, ErrPackageSvcLinkDuplicate
	}

	gatewaySvc, err := s.serviceSvc.FindByID(ctx, sid)
	if err != nil {
		return nil, ErrPackageSvcLinkNotFound
	}

	if err := validatePaths(body.Paths, gatewaySvc); err != nil {
		return nil, err
	}

	now := tool.NowUTC()
	doc := &model.PackageSvcLink{
		ID:        bson.NewObjectID(),
		PackageID: pid,
		ServiceID: sid,
		Paths:     body.Paths,
		CreatedAt: &now,
		CreatedBy: &ctxUser.ID,
		UpdatedAt: &now,
		UpdatedBy: &ctxUser.ID,
	}

	if err := s.repo.Insert(ctx, doc); err != nil {
		return nil, err
	}

	_ = s.pkgRepo.IncrServiceCount(ctx, pid, 1)
	s.syncServicePackages(ctx, gatewaySvc)

	if pkg, pkgErr := s.packageRepo.FindByID(ctx, pid); pkgErr == nil {
		s.notifyServicesUpdated(ctx, pkg, gatewaySvc.Name, "added", ctxUser.ID)
	}

	return s.populateDetail(ctx, doc), nil
}

func (s *PackageSvcLinkService) Update(ctx context.Context, packageID, serviceID string, body *model.PackageSvcLinkUpdate, ctxUser *model.User) (*model.PackageSvcLinkDetail, error) {
	pid, err := bson.ObjectIDFromHex(packageID)
	if err != nil {
		return nil, ErrPackageSvcLinkNotFound
	}
	sid, err := bson.ObjectIDFromHex(serviceID)
	if err != nil {
		return nil, ErrPackageSvcLinkNotFound
	}

	existing, err := s.repo.FindByPackageAndService(ctx, pid, sid)
	if err != nil {
		return nil, ErrPackageSvcLinkNotFound
	}

	gatewaySvc, err := s.serviceSvc.FindByID(ctx, sid)
	if err != nil {
		return nil, ErrPackageSvcLinkNotFound
	}

	if err := validatePaths(body.Paths, gatewaySvc); err != nil {
		return nil, err
	}

	now := tool.NowUTC()
	fields := bson.M{
		"paths":     body.Paths,
		"updatedAt": now,
		"updatedBy": ctxUser.ID,
	}

	if err := s.repo.Update(ctx, existing.ID, fields); err != nil {
		return nil, err
	}

	updated, err := s.repo.FindByPackageAndService(ctx, pid, sid)
	if err != nil {
		return nil, err
	}

	if pkg, pkgErr := s.packageRepo.FindByID(ctx, pid); pkgErr == nil {
		s.notifyServicesUpdated(ctx, pkg, gatewaySvc.Name, "updated", ctxUser.ID)
	}

	return s.populateDetail(ctx, updated), nil
}

func (s *PackageSvcLinkService) Delete(ctx context.Context, packageID, serviceID string, ctxUser *model.User) error {
	pid, err := bson.ObjectIDFromHex(packageID)
	if err != nil {
		return ErrPackageSvcLinkNotFound
	}
	sid, err := bson.ObjectIDFromHex(serviceID)
	if err != nil {
		return ErrPackageSvcLinkNotFound
	}

	if _, err := s.repo.FindByPackageAndService(ctx, pid, sid); err != nil {
		return ErrPackageSvcLinkNotFound
	}

	gatewaySvc, svcErr := s.serviceSvc.FindByID(ctx, sid)

	if err := s.repo.Delete(ctx, pid, sid); err != nil {
		return err
	}

	_ = s.pkgRepo.IncrServiceCount(ctx, pid, -1)

	if svcErr == nil {
		s.syncServicePackages(ctx, gatewaySvc)
		if pkg, pkgErr := s.packageRepo.FindByID(ctx, pid); pkgErr == nil {
			s.notifyServicesUpdated(ctx, pkg, gatewaySvc.Name, "removed", ctxUser.ID)
		}
	}
	return nil
}

// CheckPaths validates a list of package paths against a service's resource paths without persisting.
func (s *PackageSvcLinkService) CheckPaths(ctx context.Context, serviceID bson.ObjectID, paths []*model.PackagePath) (*model.PackageServiceCheckPathsResponse, error) {
	gatewaySvc, err := s.serviceSvc.FindByID(ctx, serviceID)
	if err != nil || gatewaySvc == nil {
		return nil, ErrPackageSvcLinkNotFound
	}

	resp := &model.PackageServiceCheckPathsResponse{}
	seen := map[string]bool{}

	for _, p := range paths {
		result := checkSinglePath(p, gatewaySvc.ResourcePaths, seen)
		if !result.Valid {
			resp.HasInvalid = true
		}
		resp.Results = append(resp.Results, result)
	}

	return resp, nil
}

func checkSinglePath(p *model.PackagePath, resourcePaths []*model.ResourcePath, seen map[string]bool) *model.PackageServiceCheckPathsResult {
	if !matchesServicePaths(p, resourcePaths) {
		return &model.PackageServiceCheckPathsResult{Path: p.Path, Valid: false, Reason: "Path or methods are not allowed by this service"}
	}
	for _, m := range p.Methods {
		key := p.Path + "::" + strings.ToUpper(m)
		if seen[key] {
			return &model.PackageServiceCheckPathsResult{Path: p.Path, Valid: false, Reason: "Duplicate path and method combination"}
		}
		seen[key] = true
	}
	return &model.PackageServiceCheckPathsResult{Path: p.Path, Valid: true}
}

// validatePaths checks that each path entry is valid against the gateway service's resourcePaths
// and that there are no duplicate (path, method) combinations.
func validatePaths(paths []*model.PackagePath, gatewaySvc *model.GatewayService) error {
	seen := map[string]bool{}
	for _, p := range paths {
		if !matchesServicePaths(p, gatewaySvc.ResourcePaths) {
			return ErrPackagePathInvalid
		}
		for _, m := range p.Methods {
			key := p.Path + "::" + strings.ToUpper(m)
			if seen[key] {
				return ErrPackagePathInvalid
			}
			seen[key] = true
		}
	}
	return nil
}

// matchesServicePaths returns true if the package path pattern matches at least one service resourcePath
// and all requested methods are supported.
func matchesServicePaths(pkgPath *model.PackagePath, resourcePaths []*model.ResourcePath) bool {
	supportedMethods := map[string]bool{}
	matched := false

	for _, rp := range resourcePaths {
		if pathCovers(pkgPath.Path, rp.Path) {
			matched = true
			for _, m := range rp.Methods {
				supportedMethods[strings.ToUpper(m)] = true
			}
		}
	}

	if !matched {
		return false
	}

	for _, m := range pkgPath.Methods {
		if !supportedMethods[strings.ToUpper(m)] {
			return false
		}
	}
	return true
}

// pathCovers returns true if pkgPath covers svcPath.
// pkgPath ending with /* matches any svcPath with the same prefix.
func pathCovers(pkgPath, svcPath string) bool {
	if strings.HasSuffix(pkgPath, "/*") {
		prefix := strings.TrimSuffix(pkgPath, "/*")
		return strings.HasPrefix(svcPath, prefix+"/")
	}
	return pkgPath == svcPath
}

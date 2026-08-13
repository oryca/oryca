package handler

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/service"
	"github.com/oryca/oryca/control-plane/tool"

	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	msgPackageNotFound          = "Not found"
	msgPackageServiceIDRequired = "Body 'serviceId' is required"
	msgPackageServiceIDInvalid  = "Body 'serviceId' is invalid"
	msgPackagePathsRequired     = "Body 'paths' is required and must not be empty"
)

type packageService interface {
	List(ctx context.Context, params url.Values) ([]*model.Package, int64, error)
	GetByID(ctx context.Context, id string) (*model.Package, error)
	Create(ctx context.Context, body *model.PackageCreate, ctxUser *model.User) (*model.Package, error)
	Update(ctx context.Context, id string, body *model.PackageUpdate, ctxUser *model.User) (*model.Package, error)
	Delete(ctx context.Context, id string, forever bool, ctxUser *model.User) error
}

type packageSvcLinkService interface {
	ListByPackage(ctx context.Context, packageID string, params url.Values) ([]*model.PackageSvcLinkDetail, int64, error)
	GetByService(ctx context.Context, packageID, serviceID string) (*model.PackageSvcLinkDetail, error)
	Create(ctx context.Context, packageID string, body *model.PackageSvcLinkCreate, ctxUser *model.User) (*model.PackageSvcLinkDetail, error)
	Update(ctx context.Context, packageID, serviceID string, body *model.PackageSvcLinkUpdate, ctxUser *model.User) (*model.PackageSvcLinkDetail, error)
	Delete(ctx context.Context, packageID, serviceID string, ctxUser *model.User) error
	CheckPaths(ctx context.Context, serviceID bson.ObjectID, paths []*model.PackagePath) (*model.PackageServiceCheckPathsResponse, error)
}

type packageUserService interface {
	GetUsers(ctx context.Context, packageID string, params url.Values) ([]*model.User, int64, error)
	AssignUsers(ctx context.Context, packageID string, rawIDs []string, groupID *string, ctxUser *model.User) (assigned int64, skipped int, err error)
	RemoveUsers(ctx context.Context, packageID string, rawIDs []string, groupID *string, ctxUser *model.User) (removed int64, skipped int, err error)
}

type PackageHandler struct {
	svc            packageService
	svcLinkSvc     packageSvcLinkService
	packageUserSvc packageUserService
}

func NewPackageHandler(svc packageService, svcLinkSvc packageSvcLinkService, packageUserSvc packageUserService) *PackageHandler {
	return &PackageHandler{svc: svc, svcLinkSvc: svcLinkSvc, packageUserSvc: packageUserSvc}
}

func (h *PackageHandler) isOwnPackage(ctxUser *model.User, packageID string) bool {
	return ctxUser.PackageID != nil && ctxUser.PackageID.Hex() == packageID
}

func (h *PackageHandler) hasWriteAccess(c echo.Context, ctxUser *model.User) bool {
	if isRootOrAdmin(ctxUser.Role) {
		return true
	}
	return false
}

func (h *PackageHandler) GetPackages(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	params := c.Request().URL.Query()

	// user role sees all enabled packages (to support package selection)
	if ctxUser.Role == "user" {
		params.Set("enabled", "true")
	}

	if v := params.Get("limit"); v != "" {
		if _, err := tool.ParseLimit(v); err != nil {
			return c.JSON(http.StatusBadRequest, &model.Exception{
				Code:   tool.CodeQueryParamInvalidFormat,
				Status: http.StatusBadRequest,
				Detail: err.Error(),
			})
		}
	}
	if v := params.Get("offset"); v != "" {
		if _, err := tool.ParseOffset(v); err != nil {
			return c.JSON(http.StatusBadRequest, &model.Exception{
				Code:   tool.CodeQueryParamInvalidFormat,
				Status: http.StatusBadRequest,
				Detail: err.Error(),
			})
		}
	}

	list, total, err := h.svc.List(c.Request().Context(), params)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not get packages",
		})
	}

	if list == nil {
		list = []*model.Package{}
	}

	return c.JSON(http.StatusOK, &model.ListResponse[*model.Package]{
		NumberMatched:  int(total),
		NumberReturned: len(list),
		Items:          list,
	})
}

func (h *PackageHandler) GetPackage(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	pkg, err := h.svc.GetByID(c.Request().Context(), c.Param("packageId"))
	if err != nil {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodePackageNotFound,
			Status: http.StatusNotFound,
			Detail: msgPackageNotFound,
		})
	}

	// user role can only see enabled packages
	if ctxUser.Role == "user" && (pkg.Enabled == nil || !*pkg.Enabled) {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodePackageNotFound,
			Status: http.StatusNotFound,
			Detail: msgPackageNotFound,
		})
	}

	return c.JSON(http.StatusOK, pkg)
}

func (h *PackageHandler) CreatePackage(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || !h.hasWriteAccess(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	var body model.PackageCreate
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	if body.Name == "" {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: "Body 'name' is required",
		})
	}

	pkg, err := h.svc.Create(c.Request().Context(), &body, ctxUser)
	if err != nil {
		if errors.Is(err, service.ErrPackageAliasDuplicate) {
			return c.JSON(http.StatusConflict, &model.Exception{
				Code:   tool.CodePackageAliasDuplicate,
				Status: http.StatusConflict,
				Detail: "Body 'alias' already exists",
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not create package",
		})
	}

	return c.JSON(http.StatusCreated, pkg)
}

func (h *PackageHandler) UpdatePackage(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || !h.hasWriteAccess(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	var body model.PackageUpdate
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	if body.Name == "" {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: "Body 'name' is required",
		})
	}
	if body.Alias == "" {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: "Body 'alias' is required",
		})
	}

	pkg, err := h.svc.Update(c.Request().Context(), c.Param("packageId"), &body, ctxUser)
	if err != nil {
		if errors.Is(err, service.ErrPackageNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodePackageNotFound,
				Status: http.StatusNotFound,
				Detail: msgPackageNotFound,
			})
		}
		if errors.Is(err, service.ErrPackageAliasDuplicate) {
			return c.JSON(http.StatusConflict, &model.Exception{
				Code:   tool.CodePackageAliasDuplicate,
				Status: http.StatusConflict,
				Detail: "Body 'alias' already exists",
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not update package",
		})
	}

	return c.JSON(http.StatusOK, pkg)
}

func (h *PackageHandler) DeletePackage(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || !h.hasWriteAccess(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	forever := c.QueryParam("forever") == "true"

	if err := h.svc.Delete(c.Request().Context(), c.Param("packageId"), forever, ctxUser); err != nil {
		if errors.Is(err, service.ErrPackageNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodePackageNotFound,
				Status: http.StatusNotFound,
				Detail: msgPackageNotFound,
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not delete package",
		})
	}

	return c.NoContent(http.StatusNoContent)
}

// === Package Services ===

func (h *PackageHandler) GetPackageServices(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	params := c.Request().URL.Query()
	if v := params.Get("limit"); v != "" {
		if _, err := tool.ParseLimit(v); err != nil {
			return c.JSON(http.StatusBadRequest, &model.Exception{
				Code:   tool.CodeQueryParamInvalidFormat,
				Status: http.StatusBadRequest,
				Detail: err.Error(),
			})
		}
	}
	if v := params.Get("offset"); v != "" {
		if _, err := tool.ParseOffset(v); err != nil {
			return c.JSON(http.StatusBadRequest, &model.Exception{
				Code:   tool.CodeQueryParamInvalidFormat,
				Status: http.StatusBadRequest,
				Detail: err.Error(),
			})
		}
	}
	list, total, err := h.svcLinkSvc.ListByPackage(c.Request().Context(), c.Param("packageId"), params)
	if err != nil {
		if errors.Is(err, service.ErrPackageNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodePackageNotFound,
				Status: http.StatusNotFound,
				Detail: msgPackageNotFound,
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not get package services",
		})
	}

	if list == nil {
		list = []*model.PackageSvcLinkDetail{}
	}

	return c.JSON(http.StatusOK, &model.ListResponse[*model.PackageSvcLinkDetail]{
		NumberMatched:  int(total),
		NumberReturned: len(list),
		Items:          list,
	})
}

func (h *PackageHandler) GetPackageService(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	if ctxUser.Role == "user" && !h.isOwnPackage(ctxUser, c.Param("packageId")) {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodePackageServiceNotFound,
			Status: http.StatusNotFound,
			Detail: msgPackageNotFound,
		})
	}

	detail, err := h.svcLinkSvc.GetByService(c.Request().Context(), c.Param("packageId"), c.Param("serviceId"))
	if err != nil {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodePackageServiceNotFound,
			Status: http.StatusNotFound,
			Detail: msgPackageNotFound,
		})
	}

	return c.JSON(http.StatusOK, detail)
}

func (h *PackageHandler) CreatePackageService(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || !h.hasWriteAccess(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	var body model.PackageSvcLinkCreate
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	if body.ServiceID == "" {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgPackageServiceIDRequired,
		})
	}
	if len(body.Paths) == 0 {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgPackagePathsRequired,
		})
	}

	detail, err := h.svcLinkSvc.Create(c.Request().Context(), c.Param("packageId"), &body, ctxUser)
	if err != nil {
		if errors.Is(err, service.ErrPackageSvcLinkDuplicate) {
			return c.JSON(http.StatusConflict, &model.Exception{
				Code:   tool.CodePackageServiceDuplicate,
				Status: http.StatusConflict,
				Detail: "Service already added to this package",
			})
		}
		if errors.Is(err, service.ErrPackagePathInvalid) {
			return c.JSON(http.StatusBadRequest, &model.Exception{
				Code:   tool.CodePackagePathInvalid,
				Status: http.StatusBadRequest,
				Detail: "One or more paths or methods are invalid for this service",
			})
		}
		if errors.Is(err, service.ErrPackageSvcLinkNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodePackageServiceNotFound,
				Status: http.StatusNotFound,
				Detail: "Gateway service not found",
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not add service to package",
		})
	}

	return c.JSON(http.StatusCreated, detail)
}

func (h *PackageHandler) UpdatePackageService(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || !h.hasWriteAccess(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	var body model.PackageSvcLinkUpdate
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	if len(body.Paths) == 0 {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgPackagePathsRequired,
		})
	}

	detail, err := h.svcLinkSvc.Update(c.Request().Context(), c.Param("packageId"), c.Param("serviceId"), &body, ctxUser)
	if err != nil {
		if errors.Is(err, service.ErrPackageSvcLinkNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodePackageServiceNotFound,
				Status: http.StatusNotFound,
				Detail: msgPackageNotFound,
			})
		}
		if errors.Is(err, service.ErrPackagePathInvalid) {
			return c.JSON(http.StatusBadRequest, &model.Exception{
				Code:   tool.CodePackagePathInvalid,
				Status: http.StatusBadRequest,
				Detail: "One or more paths or methods are invalid for this service",
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not update package service",
		})
	}

	return c.JSON(http.StatusOK, detail)
}

func (h *PackageHandler) DeletePackageService(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || !h.hasWriteAccess(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	if err := h.svcLinkSvc.Delete(c.Request().Context(), c.Param("packageId"), c.Param("serviceId"), ctxUser); err != nil {
		if errors.Is(err, service.ErrPackageSvcLinkNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodePackageServiceNotFound,
				Status: http.StatusNotFound,
				Detail: msgPackageNotFound,
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not delete package service",
		})
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *PackageHandler) CheckPackageServicePaths(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || !h.hasWriteAccess(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	var body model.PackageServiceCheckPathsRequest
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}
	if body.ServiceID == "" {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyIsRequired,
			Status: http.StatusBadRequest,
			Detail: msgPackageServiceIDRequired,
		})
	}
	if len(body.Paths) == 0 {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyIsRequired,
			Status: http.StatusBadRequest,
			Detail: msgPackagePathsRequired,
		})
	}

	serviceID, err := bson.ObjectIDFromHex(body.ServiceID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgPackageServiceIDInvalid,
		})
	}

	resp, err := h.svcLinkSvc.CheckPaths(c.Request().Context(), serviceID, body.Paths)
	if err != nil {
		if errors.Is(err, service.ErrPackageSvcLinkNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodePackageServiceNotFound,
				Status: http.StatusNotFound,
				Detail: "Service not found",
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not check paths",
		})
	}

	return c.JSON(http.StatusOK, resp)
}

// === Package Users ===

func (h *PackageHandler) GetPackageUsers(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	if ctxUser.Role == "user" && !h.isOwnPackage(ctxUser, c.Param("packageId")) {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodePackageNotFound,
			Status: http.StatusNotFound,
			Detail: msgPackageNotFound,
		})
	}

	params := c.Request().URL.Query()
	if v := params.Get("limit"); v != "" {
		if _, err := tool.ParseLimit(v); err != nil {
			return c.JSON(http.StatusBadRequest, &model.Exception{
				Code:   tool.CodeQueryParamInvalidFormat,
				Status: http.StatusBadRequest,
				Detail: err.Error(),
			})
		}
	}
	if v := params.Get("offset"); v != "" {
		if _, err := tool.ParseOffset(v); err != nil {
			return c.JSON(http.StatusBadRequest, &model.Exception{
				Code:   tool.CodeQueryParamInvalidFormat,
				Status: http.StatusBadRequest,
				Detail: err.Error(),
			})
		}
	}
	list, total, err := h.packageUserSvc.GetUsers(c.Request().Context(), c.Param("packageId"), params)
	if err != nil {
		if errors.Is(err, service.ErrPackageNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodePackageNotFound,
				Status: http.StatusNotFound,
				Detail: msgPackageNotFound,
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not get package users",
		})
	}

	if list == nil {
		list = []*model.User{}
	}

	if ctxUser.Role == "user" {
		public := make([]*model.UserPublic, len(list))
		for i, u := range list {
			public[i] = &model.UserPublic{
				ID:          u.ID,
				FirstName:   u.FirstName,
				LastName:    u.LastName,
				DisplayName: u.DisplayName,
				Email:       u.Email,
				Avatar:      u.Avatar,
				Role:        u.Role,
			}
		}
		return c.JSON(http.StatusOK, &model.ListResponse[*model.UserPublic]{
			NumberMatched:  int(total),
			NumberReturned: len(public),
			Items:          public,
		})
	}

	return c.JSON(http.StatusOK, &model.ListResponse[*model.User]{
		NumberMatched:  int(total),
		NumberReturned: len(list),
		Items:          list,
	})
}

// isSelfAssignOnly ตรวจว่า body เป็นการย้าย package ของตัวเองเท่านั้น (ห้าม groupId, ห้ามคนอื่น)
func isSelfAssignOnly(body *model.PackageUserAssign, ctxUser *model.User) bool {
	if body.GroupID != nil || len(body.UserIDs) != 1 {
		return false
	}
	return body.UserIDs[0] == ctxUser.ID.Hex()
}

// assignAccessError คืน exception ถ้า actor ไม่มีสิทธิ์ assign ตาม body ที่ส่งมา (nil = ผ่าน)
func (h *PackageHandler) assignAccessError(c echo.Context, ctxUser *model.User, body *model.PackageUserAssign) *model.Exception {
	if h.hasWriteAccess(c, ctxUser) {
		return nil
	}
	// ไม่มีสิทธิ์ package:write → ย้าย package ให้ตัวเองได้อย่างเดียว
	if !isSelfAssignOnly(body, ctxUser) {
		return &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		}
	}

	// และเลือกได้เฉพาะ package ที่เปิดใช้งาน
	pkg, err := h.svc.GetByID(c.Request().Context(), c.Param("packageId"))
	if err != nil || pkg.Enabled == nil || !*pkg.Enabled {
		return &model.Exception{
			Code:   tool.CodePackageNotFound,
			Status: http.StatusNotFound,
			Detail: msgPackageNotFound,
		}
	}
	return nil
}

func (h *PackageHandler) AssignPackageUsers(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	var body model.PackageUserAssign
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}
	if len(body.UserIDs) == 0 && body.GroupID == nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: "Body must contain 'userIds' or 'groupId'",
		})
	}

	if ex := h.assignAccessError(c, ctxUser, &body); ex != nil {
		return c.JSON(ex.Status, ex)
	}

	assigned, skipped, err := h.packageUserSvc.AssignUsers(c.Request().Context(), c.Param("packageId"), body.UserIDs, body.GroupID, ctxUser)
	if err != nil {
		if errors.Is(err, service.ErrPackageNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodePackageNotFound,
				Status: http.StatusNotFound,
				Detail: msgPackageNotFound,
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not assign users to package",
		})
	}

	return c.JSON(http.StatusOK, &model.Exception{
		Code:   tool.CodePackageUserAssigned,
		Status: http.StatusOK,
		Detail: "Users assigned to package",
		Properties: map[string]interface{}{
			"packageId": c.Param("packageId"),
			"assigned":  assigned,
			"skipped":   skipped,
		},
	})
}

func (h *PackageHandler) RemovePackageUsers(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || !h.hasWriteAccess(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	var body model.PackageUserRemove
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}
	if len(body.UserIDs) == 0 && body.GroupID == nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: "Body must contain 'userIds' or 'groupId'",
		})
	}

	removed, skipped, err := h.packageUserSvc.RemoveUsers(c.Request().Context(), c.Param("packageId"), body.UserIDs, body.GroupID, ctxUser)
	if err != nil {
		if errors.Is(err, service.ErrPackageNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodePackageNotFound,
				Status: http.StatusNotFound,
				Detail: msgPackageNotFound,
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not remove users from package",
		})
	}

	return c.JSON(http.StatusOK, &model.Exception{
		Code:   tool.CodePackageUserRemoved,
		Status: http.StatusOK,
		Detail: "Users removed from package",
		Properties: map[string]interface{}{
			"packageId": c.Param("packageId"),
			"removed":   removed,
			"skipped":   skipped,
		},
	})
}

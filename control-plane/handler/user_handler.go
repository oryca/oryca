package handler

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"net/url"
	"strings"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/service"
	"github.com/oryca/oryca/control-plane/tool"

	"github.com/labstack/echo/v4"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	msgInvalidUserID    = "Param 'userId' is invalid"
	msgUserNotFound     = "Not found"
	msgCannotDeleteSelf = "Cannot delete your own account"
)

type userService interface {
	List(ctx context.Context, params url.Values, excludeRoot bool) ([]*model.User, int64, error)
	GetByID(ctx context.Context, id bson.ObjectID) (*model.User, error)
	Create(ctx context.Context, body *model.UserCreate) (*model.User, error)
	Update(ctx context.Context, id bson.ObjectID, body *model.UserUpdate) (*model.User, error)
	Delete(ctx context.Context, id bson.ObjectID, forever bool, ctxUserID bson.ObjectID) error
	BulkDelete(ctx context.Context, body *model.UserBulkDelete, ctxUserID bson.ObjectID) error
}

type UserHandler struct {
	svc userService
}

func NewUserHandler(svc userService) *UserHandler {
	return &UserHandler{svc: svc}
}

func isRootOrAdmin(role string) bool {
	return role == "root" || role == "admin"
}

// canSetUserRole reports whether the actor may create or change a user with that role.
// root may do any role, admin only user, and nobody else may.
func canSetUserRole(actorRole, targetRole string) bool {
	switch actorRole {
	case "root":
		return true
	case "admin":
		return targetRole == "user"
	default:
		return false
	}
}

// canDeleteUser reports whether the actor may delete the target. Nobody deletes themselves, and
// nobody deletes root; otherwise root may delete anyone and admin may delete a user.
func canDeleteUser(actor, target *model.User) bool {
	if actor.ID == target.ID {
		return false
	}
	if target.Role == "root" {
		return false
	}
	switch actor.Role {
	case "root":
		return true
	case "admin":
		return target.Role == "user"
	default:
		return false
	}
}

func (h *UserHandler) GetUsers(c echo.Context) error {
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

	ctxUser, _ := c.Get("user").(*model.User)
	excludeRoot := ctxUser == nil || ctxUser.Role != "root"

	list, total, err := h.svc.List(c.Request().Context(), params, excludeRoot)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not get users",
		})
	}

	if list == nil {
		list = []*model.User{}
	}

	// the user role sees everyone in the list as UserPublic only
	if ctxUser != nil && ctxUser.Role == "user" {
		public := make([]*model.UserPublic, len(list))
		for i, u := range list {
			public[i] = &model.UserPublic{
				ID:          u.ID,
				FirstName:   u.FirstName,
				LastName:    u.LastName,
				Phone:       u.Phone,
				Username:    u.Username,
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

func (h *UserHandler) GetUser(c echo.Context) error {
	id, err := bson.ObjectIDFromHex(c.Param("userId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeParamInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgInvalidUserID,
		})
	}

	u, err := h.svc.GetByID(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodeUserNotFound,
			Status: http.StatusNotFound,
			Detail: msgUserNotFound,
		})
	}

	ctxUser, _ := c.Get("user").(*model.User)

	// anyone who is not root does not see the root user
	if u.Role == "root" && (ctxUser == nil || ctxUser.Role != "root") {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodeUserNotFound,
			Status: http.StatusNotFound,
			Detail: msgUserNotFound,
		})
	}

	// the user role sees itself in full, and everyone else as UserPublic
	if ctxUser != nil && ctxUser.Role == "user" && ctxUser.ID != u.ID {
		return c.JSON(http.StatusOK, &model.UserPublic{
			ID:          u.ID,
			FirstName:   u.FirstName,
			LastName:    u.LastName,
			Phone:       u.Phone,
			Email:       u.Email,
			Username:    u.Username,
			DisplayName: u.DisplayName,
			Avatar:      u.Avatar,
			Role:        u.Role,
		})
	}

	return c.JSON(http.StatusOK, u)
}

func (h *UserHandler) GetUserPublic(c echo.Context) error {
	id, err := bson.ObjectIDFromHex(c.Param("userId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeParamInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgInvalidUserID,
		})
	}

	u, err := h.svc.GetByID(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodeUserNotFound,
			Status: http.StatusNotFound,
			Detail: msgUserNotFound,
		})
	}

	return c.JSON(http.StatusOK, &model.UserPublic{
		ID:          u.ID,
		FirstName:   u.FirstName,
		LastName:    u.LastName,
		DisplayName: u.DisplayName,
		Avatar:      u.Avatar,
		Role:        u.Role,
	})
}

func validateUserCreateBody(body *model.UserCreate) *model.Exception {
	if body.FirstName == "" {
		return &model.Exception{Code: tool.CodeBodyIsRequired, Status: http.StatusBadRequest, Detail: "Body 'firstName' is required"}
	}
	if body.LastName == "" {
		return &model.Exception{Code: tool.CodeBodyIsRequired, Status: http.StatusBadRequest, Detail: "Body 'lastName' is required"}
	}
	if body.Email != "" {
		body.Email = strings.TrimSpace(strings.ToLower(body.Email))
		if _, err := mail.ParseAddress(body.Email); err != nil {
			return &model.Exception{Code: tool.CodeBodyInvalidFormat, Status: http.StatusBadRequest, Detail: "Body 'email' is invalid"}
		}
	}
	return nil
}

func (h *UserHandler) CreateUser(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	if !isRootOrAdmin(ctxUser.Role) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	var body model.UserCreate
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	if ex := validateUserCreateBody(&body); ex != nil {
		return c.JSON(ex.Status, ex)
	}

	// may ctxUser create a user with the role it asked for?
	targetRole := body.Role
	if targetRole == "" {
		targetRole = "user"
	}

	if !canSetUserRole(ctxUser.Role, targetRole) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	u, err := h.svc.Create(c.Request().Context(), &body)
	if err != nil {
		return c.JSON(userCreateErrResponse(err))
	}

	return c.JSON(http.StatusCreated, u)
}

func userUpdateErrResponse(err error) (int, *model.Exception) {
	switch {
	case errors.Is(err, service.ErrUserNotFound):
		return http.StatusNotFound, &model.Exception{Code: tool.CodeUserNotFound, Status: http.StatusNotFound, Detail: msgUserNotFound}
	case errors.Is(err, service.ErrInvalidRole):
		return http.StatusBadRequest, &model.Exception{Code: tool.CodeUserRoleInvalid, Status: http.StatusBadRequest, Detail: "Role must be one of: admin, user"}
	default:
		return http.StatusUnprocessableEntity, &model.Exception{Code: tool.CodeOperationFailed, Status: http.StatusUnprocessableEntity, Detail: "Could not update user"}
	}
}

func userCreateErrResponse(err error) (int, *model.Exception) {
	switch {
	case errors.Is(err, service.ErrEmailDuplicate):
		return http.StatusConflict, &model.Exception{Code: tool.CodeEmailDuplicate, Status: http.StatusConflict, Detail: "Email already exists"}
	case errors.Is(err, service.ErrUsernameDuplicate):
		return http.StatusConflict, &model.Exception{Code: tool.CodeUsernameDuplicate, Status: http.StatusConflict, Detail: "Username already exists"}
	case errors.Is(err, service.ErrPhoneDuplicate):
		return http.StatusConflict, &model.Exception{Code: tool.CodePhoneDuplicate, Status: http.StatusConflict, Detail: "Phone already exists"}
	case errors.Is(err, service.ErrInvalidRole):
		return http.StatusBadRequest, &model.Exception{Code: tool.CodeUserRoleInvalid, Status: http.StatusBadRequest, Detail: "Role must be one of: admin, user"}
	default:
		return http.StatusUnprocessableEntity, &model.Exception{Code: tool.CodeOperationFailed, Status: http.StatusUnprocessableEntity, Detail: "Could not create user"}
	}
}

// canUpdateTarget returns false if the actor is not allowed to update the target user.
func canUpdateTarget(ctxUser, target *model.User) bool {
	// an admin cannot change another admin
	if ctxUser.Role == "admin" && target.Role == "admin" && target.ID != ctxUser.ID {
		return false
	}
	return true
}

// canSetNewRole returns false if the actor is not allowed to set the given role.
func canSetNewRole(ctxUser *model.User, newRole string) bool {
	return canSetUserRole(ctxUser.Role, newRole)
}

func (h *UserHandler) UpdateUser(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	if !isRootOrAdmin(ctxUser.Role) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	id, err := bson.ObjectIDFromHex(c.Param("userId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeParamInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgInvalidUserID,
		})
	}

	target, err := h.svc.GetByID(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodeUserNotFound,
			Status: http.StatusNotFound,
			Detail: msgUserNotFound,
		})
	}

	if !canUpdateTarget(ctxUser, target) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	var body model.UserUpdate
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	newRole := body.Role
	if newRole == "" {
		newRole = target.Role
	}
	if !canSetNewRole(ctxUser, newRole) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	u, err := h.svc.Update(c.Request().Context(), id, &body)
	if err != nil {
		return c.JSON(userUpdateErrResponse(err))
	}

	return c.JSON(http.StatusOK, u)
}

func (h *UserHandler) DeleteUser(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	id, err := bson.ObjectIDFromHex(c.Param("userId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeParamInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgInvalidUserID,
		})
	}

	target, err := h.svc.GetByID(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodeUserNotFound,
			Status: http.StatusNotFound,
			Detail: msgUserNotFound,
		})
	}

	if !canDeleteUser(ctxUser, target) {
		if ctxUser.ID == target.ID {
			return c.JSON(http.StatusConflict, &model.Exception{
				Code:   tool.CodeCannotDeleteAccount,
				Status: http.StatusConflict,
				Detail: msgCannotDeleteSelf,
			})
		}
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	forever := c.QueryParam("forever") == "true"

	if err := h.svc.Delete(c.Request().Context(), id, forever, ctxUser.ID); err != nil {
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not delete user",
		})
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *UserHandler) BulkDeleteUsers(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || (ctxUser.Role != "root" && ctxUser.Role != "admin") {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	var body model.UserBulkDelete
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	if len(body.UserIDs) == 0 {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: "Body 'userIds' must not be empty",
		})
	}

	if err := h.svc.BulkDelete(c.Request().Context(), &body, ctxUser.ID); err != nil {
		if errors.Is(err, service.ErrCannotDeleteSelf) {
			return c.JSON(http.StatusConflict, &model.Exception{
				Code:   tool.CodeCannotDeleteAccount,
				Status: http.StatusConflict,
				Detail: msgCannotDeleteSelf,
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not delete users",
		})
	}
	return c.NoContent(http.StatusNoContent)
}

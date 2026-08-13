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
	msgInvalidGroupID = "Param 'groupId' is invalid"
	msgGroupNotFound  = "Not found"
)

type groupService interface {
	List(ctx context.Context, params url.Values) ([]*model.Group, int64, error)
	GetByID(ctx context.Context, id bson.ObjectID) (*model.Group, error)
	Create(ctx context.Context, body *model.GroupCreate, ctxUser *model.User) (*model.Group, error)
	Update(ctx context.Context, id bson.ObjectID, body *model.GroupUpdate, ctxUser *model.User) (*model.Group, error)
	Delete(ctx context.Context, id bson.ObjectID, forever bool, ctxUser *model.User) error
	GetGroupUsers(ctx context.Context, groupID bson.ObjectID, params url.Values) ([]*model.GroupUserDetail, int64, error)
	AddGroupUsers(ctx context.Context, groupID bson.ObjectID, userIDs []bson.ObjectID, ctxUser *model.User) (*model.GroupUserAddResult, error)
	RemoveGroupUsers(ctx context.Context, groupID bson.ObjectID, body *model.GroupUserRemove, ctxUser *model.User) error
}

type GroupHandler struct {
	svc groupService
}

func NewGroupHandler(svc groupService) *GroupHandler {
	return &GroupHandler{svc: svc}
}

func isRootOrAdmin(role string) bool {
	return role == "root" || role == "admin"
}

func (h *GroupHandler) hasGroupWriteAccess(c echo.Context, ctxUser *model.User) bool {
	if isRootOrAdmin(ctxUser.Role) {
		return true
	}
	return false
}

func (h *GroupHandler) GetGroups(c echo.Context) error {
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

	list, total, err := h.svc.List(c.Request().Context(), params)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not get groups",
		})
	}

	if list == nil {
		list = []*model.Group{}
	}

	return c.JSON(http.StatusOK, &model.ListResponse[*model.Group]{
		NumberMatched:  int(total),
		NumberReturned: len(list),
		Items:          list,
	})
}

func (h *GroupHandler) GetGroup(c echo.Context) error {
	id, err := bson.ObjectIDFromHex(c.Param("groupId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeParamInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgInvalidGroupID,
		})
	}

	g, err := h.svc.GetByID(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodeGroupNotFound,
			Status: http.StatusNotFound,
			Detail: msgGroupNotFound,
		})
	}

	return c.JSON(http.StatusOK, g)
}

func (h *GroupHandler) CreateGroup(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || !h.hasGroupWriteAccess(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	var body model.GroupCreate
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

	g, err := h.svc.Create(c.Request().Context(), &body, ctxUser)
	if err != nil {
		if errors.Is(err, service.ErrGroupAliasDuplicate) {
			return c.JSON(http.StatusConflict, &model.Exception{
				Code:   tool.CodeGroupAliasDuplicate,
				Status: http.StatusConflict,
				Detail: "Group alias already exists",
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not create group",
		})
	}

	return c.JSON(http.StatusCreated, g)
}

func (h *GroupHandler) UpdateGroup(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || !h.hasGroupWriteAccess(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	id, err := bson.ObjectIDFromHex(c.Param("groupId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeParamInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgInvalidGroupID,
		})
	}

	var body model.GroupUpdate
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeBodyInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
	}

	g, err := h.svc.Update(c.Request().Context(), id, &body, ctxUser)
	if err != nil {
		if errors.Is(err, service.ErrGroupNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodeGroupNotFound,
				Status: http.StatusNotFound,
				Detail: msgGroupNotFound,
			})
		}
		if errors.Is(err, service.ErrGroupAliasDuplicate) {
			return c.JSON(http.StatusConflict, &model.Exception{
				Code:   tool.CodeGroupAliasDuplicate,
				Status: http.StatusConflict,
				Detail: "Group alias already exists",
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not update group",
		})
	}

	return c.JSON(http.StatusOK, g)
}

func (h *GroupHandler) DeleteGroup(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || !h.hasGroupWriteAccess(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	id, err := bson.ObjectIDFromHex(c.Param("groupId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeParamInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgInvalidGroupID,
		})
	}

	forever := c.QueryParam("forever") == "true"

	if err := h.svc.Delete(c.Request().Context(), id, forever, ctxUser); err != nil {
		if errors.Is(err, service.ErrGroupNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodeGroupNotFound,
				Status: http.StatusNotFound,
				Detail: msgGroupNotFound,
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not delete group",
		})
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *GroupHandler) GetGroupUsers(c echo.Context) error {
	id, err := bson.ObjectIDFromHex(c.Param("groupId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeParamInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgInvalidGroupID,
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

	list, total, err := h.svc.GetGroupUsers(c.Request().Context(), id, params)
	if err != nil {
		if errors.Is(err, service.ErrGroupNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodeGroupNotFound,
				Status: http.StatusNotFound,
				Detail: msgGroupNotFound,
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not get group users",
		})
	}

	if list == nil {
		list = []*model.GroupUserDetail{}
	}

	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser != nil && ctxUser.Role == "user" {
		public := make([]*model.GroupUserPublicDetail, len(list))
		for i, item := range list {
			public[i] = &model.GroupUserPublicDetail{
				ID: item.ID,
				User: model.UserPublic{
					ID:          item.User.ID,
					FirstName:   item.User.FirstName,
					LastName:    item.User.LastName,
					DisplayName: item.User.DisplayName,
					Email:       item.User.Email,
					Avatar:      item.User.Avatar,
					Role:        item.User.Role,
				},
				Group:     item.Group,
				CreatedAt: item.CreatedAt,
				CreatedBy: item.CreatedBy,
				UpdatedAt: item.UpdatedAt,
				UpdatedBy: item.UpdatedBy,
			}
		}
		return c.JSON(http.StatusOK, &model.ListResponse[*model.GroupUserPublicDetail]{
			NumberMatched:  int(total),
			NumberReturned: len(public),
			Items:          public,
		})
	}

	return c.JSON(http.StatusOK, &model.ListResponse[*model.GroupUserDetail]{
		NumberMatched:  int(total),
		NumberReturned: len(list),
		Items:          list,
	})
}

func (h *GroupHandler) AddGroupUsers(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || !h.hasGroupWriteAccess(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	id, err := bson.ObjectIDFromHex(c.Param("groupId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeParamInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgInvalidGroupID,
		})
	}

	var body model.GroupUserAdd
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

	added, err := h.svc.AddGroupUsers(c.Request().Context(), id, body.UserIDs, ctxUser)
	if err != nil {
		if errors.Is(err, service.ErrGroupNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodeGroupNotFound,
				Status: http.StatusNotFound,
				Detail: msgGroupNotFound,
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not add users to group",
		})
	}

	return c.JSON(http.StatusCreated, &model.Exception{
		Code:   tool.CodeGroupUserAdded,
		Status: http.StatusCreated,
		Detail: "Users added to group successfully",
		Properties: map[string]interface{}{
			"groupId": id.Hex(),
			"added":   added.Added,
			"skipped": added.Skipped,
		},
	})
}

func (h *GroupHandler) RemoveGroupUsers(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil || !h.hasGroupWriteAccess(c, ctxUser) {
		return c.JSON(http.StatusForbidden, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusForbidden,
			Detail: msgNoPermission,
		})
	}

	id, err := bson.ObjectIDFromHex(c.Param("groupId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeParamInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgInvalidGroupID,
		})
	}

	var body model.GroupUserRemove
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

	if err := h.svc.RemoveGroupUsers(c.Request().Context(), id, &body, ctxUser); err != nil {
		if errors.Is(err, service.ErrGroupNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodeGroupNotFound,
				Status: http.StatusNotFound,
				Detail: msgGroupNotFound,
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not remove users from group",
		})
	}

	return c.NoContent(http.StatusNoContent)
}

package handler

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/service"
	"github.com/oryca/oryca/control-plane/tool"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	msgInvalidNotificationID = "Param 'notificationId' is invalid"
	msgNotificationNotFound  = "Not found"
	sseHeartbeatInterval     = 20 * time.Second
)

type notificationService interface {
	List(ctx context.Context, userID bson.ObjectID, params url.Values) ([]*model.Notification, int64, error)
	UnreadCount(ctx context.Context, userID bson.ObjectID) (int64, error)
	MarkAsRead(ctx context.Context, id bson.ObjectID, userID bson.ObjectID) error
	MarkAllAsRead(ctx context.Context, userID bson.ObjectID) error
	Delete(ctx context.Context, id bson.ObjectID, userID bson.ObjectID) error
	IssueStreamTicket(ctx context.Context, userID bson.ObjectID) (string, error)
	VerifyStreamTicket(ctx context.Context, ticket string) (bson.ObjectID, error)
	SubscribeUser(ctx context.Context, userID bson.ObjectID) *redis.PubSub
}

type NotificationHandler struct {
	svc notificationService
}

func NewNotificationHandler(svc notificationService) *NotificationHandler {
	return &NotificationHandler{svc: svc}
}

func (h *NotificationHandler) GetNotifications(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusUnauthorized, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusUnauthorized,
			Detail: "Authentication required",
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

	list, total, err := h.svc.List(c.Request().Context(), ctxUser.ID, params)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not get notifications",
		})
	}

	if list == nil {
		list = []*model.Notification{}
	}

	return c.JSON(http.StatusOK, &model.ListResponse[*model.Notification]{
		NumberMatched:  int(total),
		NumberReturned: len(list),
		Items:          list,
	})
}

func (h *NotificationHandler) GetUnreadCount(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusUnauthorized, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusUnauthorized,
			Detail: "Authentication required",
		})
	}

	count, err := h.svc.UnreadCount(c.Request().Context(), ctxUser.ID)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not get unread count",
		})
	}

	return c.JSON(http.StatusOK, &model.NotificationUnreadCount{Count: count})
}

func (h *NotificationHandler) MarkAsRead(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusUnauthorized, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusUnauthorized,
			Detail: "Authentication required",
		})
	}

	id, err := bson.ObjectIDFromHex(c.Param("notificationId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeParamInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgInvalidNotificationID,
		})
	}

	if err := h.svc.MarkAsRead(c.Request().Context(), id, ctxUser.ID); err != nil {
		if errors.Is(err, service.ErrNotificationNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodeNotFound,
				Status: http.StatusNotFound,
				Detail: msgNotificationNotFound,
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not mark notification as read",
		})
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *NotificationHandler) MarkAllAsRead(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusUnauthorized, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusUnauthorized,
			Detail: "Authentication required",
		})
	}

	if err := h.svc.MarkAllAsRead(c.Request().Context(), ctxUser.ID); err != nil {
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not mark notifications as read",
		})
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *NotificationHandler) DeleteNotification(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusUnauthorized, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusUnauthorized,
			Detail: "Authentication required",
		})
	}

	id, err := bson.ObjectIDFromHex(c.Param("notificationId"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.Exception{
			Code:   tool.CodeParamInvalidFormat,
			Status: http.StatusBadRequest,
			Detail: msgInvalidNotificationID,
		})
	}

	if err := h.svc.Delete(c.Request().Context(), id, ctxUser.ID); err != nil {
		if errors.Is(err, service.ErrNotificationNotFound) {
			return c.JSON(http.StatusNotFound, &model.Exception{
				Code:   tool.CodeNotFound,
				Status: http.StatusNotFound,
				Detail: msgNotificationNotFound,
			})
		}
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not delete notification",
		})
	}

	return c.NoContent(http.StatusNoContent)
}

// IssueStreamTicket ออกตั๋วอายุ 10 วิให้ FE เอาไปต่อ SSE
func (h *NotificationHandler) IssueStreamTicket(c echo.Context) error {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return c.JSON(http.StatusUnauthorized, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusUnauthorized,
			Detail: "Authentication required",
		})
	}

	ticket, err := h.svc.IssueStreamTicket(c.Request().Context(), ctxUser.ID)
	if err != nil {
		return c.JSON(http.StatusUnprocessableEntity, &model.Exception{
			Code:   tool.CodeOperationFailed,
			Status: http.StatusUnprocessableEntity,
			Detail: "Could not issue stream ticket",
		})
	}

	return c.JSON(http.StatusOK, map[string]string{"ticket": ticket})
}

// Stream เปิด SSE connection — auth ผ่าน ticket ใน query param ไม่ใช่ JWT header
func (h *NotificationHandler) Stream(c echo.Context) error {
	ticket := c.QueryParam("ticket")
	if ticket == "" {
		return c.JSON(http.StatusUnauthorized, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusUnauthorized,
			Detail: "Query param 'ticket' is required",
		})
	}

	ctx := c.Request().Context()
	userID, err := h.svc.VerifyStreamTicket(ctx, ticket)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusUnauthorized,
			Detail: "Invalid or expired ticket",
		})
	}

	res := c.Response()
	res.Header().Set(echo.HeaderContentType, "text/event-stream")
	res.Header().Set("Cache-Control", "no-cache")
	res.Header().Set("Connection", "keep-alive")
	res.Header().Set("X-Accel-Buffering", "no")
	res.WriteHeader(http.StatusOK)

	flusher, ok := res.Writer.(http.Flusher)
	if !ok {
		return errors.New("streaming unsupported")
	}

	pubsub := h.svc.SubscribeUser(ctx, userID)
	defer pubsub.Close()
	msgCh := pubsub.Channel()

	heartbeat := time.NewTicker(sseHeartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-msgCh:
			if !ok {
				return nil
			}
			if _, err := res.Write([]byte("event: notification\ndata: " + msg.Payload + "\n\n")); err != nil {
				return nil
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := res.Write([]byte(": heartbeat\n\n")); err != nil {
				return nil
			}
			flusher.Flush()
		}
	}
}

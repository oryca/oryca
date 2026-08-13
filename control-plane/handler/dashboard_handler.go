package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/service"
	"github.com/oryca/oryca/control-plane/tool"

	"github.com/labstack/echo/v4"
)

const (
	msgDashboardRangeInvalid = "Query 'from'/'to' is invalid"
	msgDashboardFailed       = "Could not read request logs"
)

type dashboardService interface {
	Stats(ctx context.Context, q model.AccessLogQuery, actor *model.User) (*model.DashboardStats, error)
	Summary(ctx context.Context, q model.AccessLogQuery, actor *model.User) (*model.DashboardSummary, error)
	Logs(ctx context.Context, q model.AccessLogQuery, actor *model.User) (*model.AccessLogPage, error)
}

type DashboardHandler struct {
	svc dashboardService
}

func NewDashboardHandler(svc dashboardService) *DashboardHandler {
	return &DashboardHandler{svc: svc}
}

// parseDashboardQuery reads the shared filter. Times are RFC3339; anything
// unparseable is rejected rather than silently ignored, so a typo in a
// dashboard URL cannot quietly widen the range.
func parseDashboardQuery(c echo.Context) (model.AccessLogQuery, bool) {
	q := model.AccessLogQuery{
		ServiceID: c.QueryParam("serviceId"),
		UserID:    c.QueryParam("userId"),
	}
	if raw := c.QueryParam("from"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return q, false
		}
		q.From = t.UTC()
	}
	if raw := c.QueryParam("to"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return q, false
		}
		q.To = t.UTC()
	}
	return q, true
}

func dashboardBadRange(c echo.Context) error {
	return c.JSON(http.StatusBadRequest, &model.Exception{
		Code:   tool.CodeParamInvalidFormat,
		Status: http.StatusBadRequest,
		Detail: msgDashboardRangeInvalid,
	})
}

func dashboardFailed(c echo.Context) error {
	return c.JSON(http.StatusInternalServerError, &model.Exception{
		Code:   tool.CodeOperationFailed,
		Status: http.StatusInternalServerError,
		Detail: msgDashboardFailed,
	})
}

func dashboardActor(c echo.Context) (*model.User, error) {
	ctxUser, _ := c.Get("user").(*model.User)
	if ctxUser == nil {
		return nil, c.JSON(http.StatusUnauthorized, &model.Exception{
			Code:   tool.CodeUnauthorizedAccess,
			Status: http.StatusUnauthorized,
			Detail: msgAuthRequired,
		})
	}
	return ctxUser, nil
}

// GetStats returns the request-volume series for the chart.
func (h *DashboardHandler) GetStats(c echo.Context) error {
	ctxUser, resp := dashboardActor(c)
	if ctxUser == nil {
		return resp
	}
	q, ok := parseDashboardQuery(c)
	if !ok {
		return dashboardBadRange(c)
	}

	stats, err := h.svc.Stats(c.Request().Context(), q, ctxUser)
	if err != nil {
		if errors.Is(err, service.ErrDashboardRangeInvalid) {
			return dashboardBadRange(c)
		}
		return dashboardFailed(c)
	}
	return c.JSON(http.StatusOK, stats)
}

// GetSummary returns the totals and the busiest services for the range.
func (h *DashboardHandler) GetSummary(c echo.Context) error {
	ctxUser, resp := dashboardActor(c)
	if ctxUser == nil {
		return resp
	}
	q, ok := parseDashboardQuery(c)
	if !ok {
		return dashboardBadRange(c)
	}

	summary, err := h.svc.Summary(c.Request().Context(), q, ctxUser)
	if err != nil {
		if errors.Is(err, service.ErrDashboardRangeInvalid) {
			return dashboardBadRange(c)
		}
		return dashboardFailed(c)
	}
	return c.JSON(http.StatusOK, summary)
}

// GetLogs returns the raw request table, newest first.
func (h *DashboardHandler) GetLogs(c echo.Context) error {
	ctxUser, resp := dashboardActor(c)
	if ctxUser == nil {
		return resp
	}
	q, ok := parseDashboardQuery(c)
	if !ok {
		return dashboardBadRange(c)
	}
	if raw := c.QueryParam("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit <= 0 {
			return c.JSON(http.StatusBadRequest, &model.Exception{
				Code:   tool.CodeParamInvalidFormat,
				Status: http.StatusBadRequest,
				Detail: "Query 'limit' is invalid",
			})
		}
		q.Limit = limit
	}
	if raw := c.QueryParam("cursor"); raw != "" {
		cursorTime, cursorID, valid := service.DecodeLogCursor(raw)
		if !valid {
			return c.JSON(http.StatusBadRequest, &model.Exception{
				Code:   tool.CodeParamInvalidFormat,
				Status: http.StatusBadRequest,
				Detail: "Query 'cursor' is invalid",
			})
		}
		q.CursorTime, q.CursorID = cursorTime, cursorID
	}

	page, err := h.svc.Logs(c.Request().Context(), q, ctxUser)
	if err != nil {
		if errors.Is(err, service.ErrDashboardRangeInvalid) {
			return dashboardBadRange(c)
		}
		return dashboardFailed(c)
	}
	return c.JSON(http.StatusOK, page)
}

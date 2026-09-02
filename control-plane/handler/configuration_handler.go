package handler

import (
	"context"
	"net/http"

	"github.com/oryca/oryca/control-plane/model"
	"github.com/oryca/oryca/control-plane/tool"

	"github.com/labstack/echo/v4"
)

type configurationService interface {
	Get(ctx context.Context) (*model.Configuration, error)
	Upsert(ctx context.Context, body *model.ConfigurationUpsert) (*model.Configuration, error)
}

type ConfigurationHandler struct {
	svc configurationService
}

func NewConfigurationHandler(svc configurationService) *ConfigurationHandler {
	return &ConfigurationHandler{svc: svc}
}

func (h *ConfigurationHandler) GetConfiguration(c echo.Context) error {
	cfg, err := h.svc.Get(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusNotFound, &model.Exception{
			Code:   tool.CodeConfigNotFound,
			Status: http.StatusNotFound,
			Detail: "Not found",
		})
	}

	user, _ := c.Get("user").(*model.User)
	if user != nil && user.Role == "root" {
		return c.JSON(http.StatusOK, cfg)
	}

	return c.JSON(http.StatusOK, &model.ConfigurationPublic{
		ID:            cfg.ID,
		Terms:         cfg.Terms,
		PrivacyPolicy: cfg.PrivacyPolicy,
		Register:      cfg.Register,
		UpdatedAt:     cfg.UpdatedAt,
	})
}

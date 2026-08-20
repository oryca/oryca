package service

import (
	"context"
	"testing"

	"github.com/oryca/oryca/control-plane/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMailServiceSendTemplateMailDisabled(t *testing.T) {
	svc := NewMailService(&config.SMTPConfig{})

	err := svc.SendTemplate(context.Background(), "user@example.com", aliasVerifyEmail, nil)
	assert.ErrorIs(t, err, ErrMailDisabled)
}

func TestMailServiceSendTemplateUnknownAlias(t *testing.T) {
	svc := NewMailService(&config.SMTPConfig{Host: "smtp.example.com", Port: "587"})

	err := svc.SendTemplate(context.Background(), "user@example.com", "oryca.no-such-template", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUnknownTemplate)
}

package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetMailTemplate(t *testing.T) {
	for _, alias := range []string{"oryca.verify-email", "oryca.set-password"} {
		tmpl, err := getMailTemplate(alias)
		require.NoError(t, err, "alias %s should be embedded", alias)
		assert.NotEmpty(t, tmpl.Subject)
		assert.True(t, strings.HasPrefix(tmpl.Html, "<!doctype html>"))
		assert.Contains(t, tmpl.Html, "{{name}}")
		assert.Contains(t, tmpl.Html, "{{link}}")
	}
}

func TestGetMailTemplateUnknownAlias(t *testing.T) {
	_, err := getMailTemplate("oryca.no-such-template")
	assert.ErrorIs(t, err, errUnknownTemplate)
}

func TestRenderMailTemplateVariables(t *testing.T) {
	tmpl, err := getMailTemplate("oryca.verify-email")
	require.NoError(t, err)

	body := renderVariables(tmpl.Html, map[string]string{
		"name": "Alice",
		"link": "https://oryca.test/verify?token=abc",
	})

	assert.Contains(t, body, "Hi Alice,")
	assert.Contains(t, body, "https://oryca.test/verify?token=abc")
	assert.NotContains(t, body, "{{name}}")
	assert.NotContains(t, body, "{{link}}")
}

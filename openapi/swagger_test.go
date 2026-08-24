package openapi

import (
	"strings"
	"testing"
)

func TestSetSwaggerTemplate(t *testing.T) {
	t.Cleanup(func() { SetSwaggerTemplate("") })

	SetSwaggerTemplate(`<html><body>{SwaggerUIDocUrl}</body></html>`)
	if got := string(RenderSwaggerHTML(`/docs?query=one&two`)); got != `<html><body>/docs?query=one&amp;two</body></html>` {
		t.Fatalf("custom Swagger template output=%q", got)
	}

	SetSwaggerTemplate("")
	defaultPage := string(RenderSwaggerHTML("/openapi.json"))
	if !strings.Contains(defaultPage, `id="openapi-ui-container"`) || !strings.Contains(defaultPage, "openapi-ui-dist@latest") {
		t.Fatalf("built-in Swagger template was not restored")
	}
}

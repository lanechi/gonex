package openapi

import (
	"html"
	"strings"
	"sync"
)

const defaultSwaggerHTML = `
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <title>openAPI UI</title>
  </head>
  <body>
    <div id="openapi-ui-container" spec-url="{SwaggerUIDocUrl}" theme="light"></div>
    <script src="https://cdn.jsdelivr.net/npm/openapi-ui-dist@latest/lib/openapi-ui.umd.js"></script>
  </body>
</html>
`

var swaggerTemplateState struct {
	sync.RWMutex
	template string
}

func init() {
	swaggerTemplateState.template = defaultSwaggerHTML
}

// SetSwaggerTemplate sets the process-wide Swagger UI HTML template.
//
// The template should contain the {SwaggerUIDocUrl} placeholder, which is
// replaced with the configured OpenAPI document URL when the page is rendered.
// Passing an empty string restores the built-in system template.
func SetSwaggerTemplate(template string) {
	if template == "" {
		template = defaultSwaggerHTML
	}
	swaggerTemplateState.Lock()
	swaggerTemplateState.template = template
	swaggerTemplateState.Unlock()
}

// RenderSwaggerHTML renders the configured OpenAPI UI page.
func RenderSwaggerHTML(specURL string) []byte {
	specURL = html.EscapeString(specURL)
	swaggerTemplateState.RLock()
	template := swaggerTemplateState.template
	swaggerTemplateState.RUnlock()
	return []byte(strings.Replace(template, "{SwaggerUIDocUrl}", specURL, 1))
}

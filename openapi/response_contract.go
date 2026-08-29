package openapi

import (
	"reflect"
	"sync"
)

// ResponseSchemaProvider supplies documented success and error response schemas
// independently from the runtime ResponseEncoder. Applications that customize
// the runtime response envelope can register a matching provider for OpenAPI.
type ResponseSchemaProvider interface {
	SuccessSchema(reflect.Type, []string) map[string]any
	ErrorSchemas() map[string]any
}

type defaultResponseSchemaProvider struct{}

func (defaultResponseSchemaProvider) SuccessSchema(responseType reflect.Type, produces []string) map[string]any {
	return responseSchema(responseType, produces)
}

func (defaultResponseSchemaProvider) ErrorSchemas() map[string]any {
	return map[string]any{
		"400": map[string]any{"description": "Bad Request"},
		"500": map[string]any{"description": "Internal Server Error"},
	}
}

var responseSchemaState struct {
	sync.RWMutex
	provider ResponseSchemaProvider
}

func init() {
	responseSchemaState.provider = defaultResponseSchemaProvider{}
}

// SetResponseSchemaProvider sets the process-wide OpenAPI response-schema
// provider. Passing nil restores the default gonex response envelope.
func SetResponseSchemaProvider(provider ResponseSchemaProvider) {
	if provider == nil {
		provider = defaultResponseSchemaProvider{}
	}
	responseSchemaState.Lock()
	responseSchemaState.provider = provider
	responseSchemaState.Unlock()
}

func currentResponseSchemaProvider() ResponseSchemaProvider {
	responseSchemaState.RLock()
	provider := responseSchemaState.provider
	responseSchemaState.RUnlock()
	return provider
}

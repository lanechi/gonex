package openapi

// Clone returns a deep copy suitable for returning cached documents safely.
func Clone(document Document) Document {
	clone := Document{OpenAPI: document.OpenAPI, Info: document.Info, Paths: make(map[string]map[string]Operation, len(document.Paths))}
	for path, methods := range document.Paths {
		clone.Paths[path] = make(map[string]Operation, len(methods))
		for method, operation := range methods {
			operation.Tags = append([]string(nil), operation.Tags...)
			operation.Security = append([]map[string][]string(nil), operation.Security...)
			for index := range operation.Security {
				security := make(map[string][]string, len(operation.Security[index]))
				for name, scopes := range operation.Security[index] {
					security[name] = append([]string(nil), scopes...)
				}
				operation.Security[index] = security
			}
			operation.Parameters = cloneMapSlice(operation.Parameters)
			operation.RequestBody = cloneMap(operation.RequestBody)
			operation.Responses = cloneMap(operation.Responses)
			clone.Paths[path][method] = operation
		}
	}
	if document.Components != nil {
		clone.Components = cloneMap(document.Components)
	}
	return clone
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	clone := make(map[string]any, len(value))
	for key, item := range value {
		clone[key] = cloneValue(item)
	}
	return clone
}

func cloneMapSlice(values []map[string]any) []map[string]any {
	if values == nil {
		return nil
	}
	clone := make([]map[string]any, len(values))
	for index, value := range values {
		clone[index] = cloneMap(value)
	}
	return clone
}

func cloneValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		return cloneMap(value)
	case []any:
		clone := make([]any, len(value))
		for index, item := range value {
			clone[index] = cloneValue(item)
		}
		return clone
	case []string:
		return append([]string(nil), value...)
	case []map[string]any:
		return cloneMapSlice(value)
	default:
		return value
	}
}

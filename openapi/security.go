package openapi

import "strings"

func securityRequirements(security []string) []map[string][]string {
	if len(security) == 0 {
		return nil
	}
	result := make([]map[string][]string, 0, len(security))
	for _, item := range security {
		name, scopes, hasScopes := strings.Cut(item, ":")
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		values := []string(nil)
		if hasScopes {
			values = splitTag(scopes)
		}
		result = append(result, map[string][]string{name: values})
	}
	return result
}

func addSecuritySchemes(components map[string]any, security []string) {
	if len(security) == 0 {
		return
	}
	schemes, _ := components["securitySchemes"].(map[string]any)
	if schemes == nil {
		schemes = make(map[string]any)
		components["securitySchemes"] = schemes
	}
	for _, item := range security {
		name, _, _ := strings.Cut(item, ":")
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, exists := schemes[name]; !exists {
			schemes[name] = map[string]any{"type": "http", "scheme": "bearer", "bearerFormat": "JWT"}
		}
	}
}

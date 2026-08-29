package openapi

import (
	"reflect"
	"strings"
	"testing"

	"github.com/lanechi/gonex/router"
)

type Meta struct {
	Value string `json:"value"`
}

func TestAddSchemaComponentsDisambiguatesConflictingTypeNames(t *testing.T) {
	components := map[string]any{"schemas": map[string]any{}}
	addSchemaComponents(components, reflect.TypeOf(router.Meta{}))
	addSchemaComponents(components, reflect.TypeOf(Meta{}))

	schemas := components["schemas"].(map[string]any)
	if len(schemas) != 2 {
		t.Fatalf("schemas=%v", schemas)
	}
	if _, ok := schemas["Meta"]; !ok {
		t.Fatalf("original component name was not preserved: %v", schemas)
	}
	foundDisambiguated := false
	for key := range schemas {
		if strings.HasPrefix(key, "Meta_") {
			foundDisambiguated = true
			break
		}
	}
	if !foundDisambiguated {
		t.Fatalf("conflicting component was not disambiguated: %v", schemas)
	}
}

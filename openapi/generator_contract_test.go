package openapi

import (
	"testing"

	"github.com/lanechi/gonex/router"
)

func TestGenerateUsesExplicitInfoAndDocumentsUnspecifiedErrors(t *testing.T) {
	document := Generate(Info{Title: "Orders API", Version: "2026.08"}, []router.RouteMetadata{{Path: "/orders", Method: "GET"}})
	if document.Info.Title != "Orders API" || document.Info.Version != "2026.08" {
		t.Fatalf("OpenAPI info = %#v", document.Info)
	}
	responses := document.Paths["/orders"]["get"].Responses
	if _, ok := responses["default"]; !ok {
		t.Fatalf("responses do not cover application-defined HTTP errors: %#v", responses)
	}
}

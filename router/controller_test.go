package router_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/lanechi/gonex/g"
	"github.com/lanechi/gonex/router"
)

type scannerRequest struct {
	g.Meta `path:"/scanner" method:"GET"`
}

// Meta intentionally has the same name as the framework marker but a
// different package-qualified type identity.
type Meta struct{}

type sameNamedMetaRequest struct {
	Meta `path:"/same-named-meta" method:"GET"`
}

type pointerMetaRequest struct {
	*g.Meta `path:"/pointer-meta" method:"GET"`
}

func TestParseMetaUsesTypeIdentity(t *testing.T) {
	if _, err := router.ParseMeta(reflect.TypeOf((*sameNamedMetaRequest)(nil))); err == nil {
		t.Fatal("ParseMeta() accepted a same-named non-framework Meta type")
	}
	if metadata, err := router.ParseMeta(reflect.TypeOf((*pointerMetaRequest)(nil))); err != nil {
		t.Fatalf("ParseMeta() rejected *g.Meta: %v", err)
	} else if metadata.Path != "/pointer-meta" {
		t.Fatalf("metadata path = %q, want /pointer-meta", metadata.Path)
	}
}

type scannerController struct{}

func (*scannerController) Route(context.Context, *scannerRequest) error { return nil }

func (*scannerController) Helper() string { return "helper" }

func (*scannerController) OtherHelper(value string) string { return value }

func TestScanControllerSkipsExportedHelpers(t *testing.T) {
	routes, err := router.ScanController(&scannerController{})
	if err != nil {
		t.Fatalf("ScanController() error = %v", err)
	}
	if len(routes) != 1 || routes[0].Metadata.Action != "Route" {
		t.Fatalf("routes = %#v, want only Route", routes)
	}
}

type invalidScannerController struct{}

type invalidScannerRequest struct {
	g.Meta `path:"/invalid" method:"GET"`
}

func (*invalidScannerController) Invalid(context.Context, *invalidScannerRequest, string) error {
	return nil
}

func TestScanControllerStillRejectsMalformedRouteActions(t *testing.T) {
	_, err := router.ScanController(&invalidScannerController{})
	if err == nil || !strings.Contains(err.Error(), "Invalid") {
		t.Fatalf("ScanController() error = %v, want malformed route action error", err)
	}
}

type invalidContextController struct{}

func (*invalidContextController) Invalid(string, *invalidScannerRequest) error { return nil }

func TestScanControllerRejectsRouteActionWithWrongContext(t *testing.T) {
	_, err := router.ScanController(&invalidContextController{})
	if err == nil || !strings.Contains(err.Error(), "first argument must be context.Context") {
		t.Fatalf("ScanController() error = %v, want context contract error", err)
	}
}

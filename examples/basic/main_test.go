package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lanechi/gonex/g"
	"github.com/lanechi/gonex/ghttp"
)

type groupedPathReq struct {
	g.Meta `path:"/items" method:"get"`
	Tenant string `path:"tenant"`
}

type groupedPathRes struct {
	Tenant string `json:"tenant"`
}

type groupedPathController struct{}

func (*groupedPathController) List(_ context.Context, request *groupedPathReq) (*groupedPathRes, error) {
	return &groupedPathRes{Tenant: request.Tenant}, nil
}

func TestRouterGroupPrefixParticipatesInPathBinding(t *testing.T) {
	server := ghttp.NewServer()
	server.Group("/tenants/:tenant", func(group *ghttp.RouterGroup) {
		if err := group.Bind(&groupedPathController{}); err != nil {
			t.Fatalf("group.Bind() error = %v", err)
		}
	})

	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/tenants/acme/items", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"tenant":"acme"`) {
		t.Fatalf("group path binding: status=%d body=%s", response.Code, response.Body.String())
	}
}

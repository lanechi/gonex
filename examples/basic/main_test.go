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

type defaultQueryReq struct {
	g.Meta `path:"/defaults" method:"get"`
	Page   int `query:"page" d:"1"`
}

type defaultQueryRes struct {
	Page int `json:"page"`
}

type defaultQueryController struct{}

func (*defaultQueryController) List(_ context.Context, request *defaultQueryReq) (*defaultQueryRes, error) {
	return &defaultQueryRes{Page: request.Page}, nil
}

func TestOptionalQueryParameterUsesDefault(t *testing.T) {
	server := ghttp.NewServer()
	if err := server.Bind(&defaultQueryController{}); err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/defaults", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"page":1`) {
		t.Fatalf("default query parameter: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestNamedSliceResponseValue(t *testing.T) {
	server := ghttp.NewServer()
	if err := server.Bind(&HelloController{}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/names", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"data":["gonex","gopher"]`) {
		t.Fatalf("named slice response: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestBackgroundSchedulerJobIsRegistered(t *testing.T) {
	server := ghttp.NewServer()
	if err := registerBackgroundJobs(server); err != nil {
		t.Fatal(err)
	}
	jobs := server.Scheduler().Jobs()
	if len(jobs) != 1 || jobs[0].Name != "basic-example-heartbeat" {
		t.Fatalf("scheduler jobs = %#v", jobs)
	}
}

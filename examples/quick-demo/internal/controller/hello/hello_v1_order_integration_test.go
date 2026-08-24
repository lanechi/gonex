package hello_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lanechi/gonex/examples/quick-demo/internal/controller/hello"
	_ "github.com/lanechi/gonex/examples/quick-demo/internal/logic"
	"github.com/lanechi/gonex/examples/quick-demo/internal/model"
	"github.com/lanechi/gonex/examples/quick-demo/internal/service"
	"github.com/lanechi/gonex/ghttp"
)

func TestOrderControllerUsesRegisteredServiceAndValidatesRequests(t *testing.T) {
	if service.Testservice() == nil {
		t.Fatal("testservice logic was not registered")
	}

	server := ghttp.NewServer()
	server.Group("/hello", func(group *ghttp.RouterGroup) {
		if err := group.Bind(hello.NewV1()); err != nil {
			t.Fatal(err)
		}
	})

	success := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/hello/order?source=web", bytes.NewBufferString(`{"customerName":"Lane","quantity":2}`))
	request.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(success, request)
	if success.Code != http.StatusOK {
		t.Fatalf("success status=%d body=%s", success.Code, success.Body.String())
	}
	var response struct {
		Code int `json:"code"`
		Data struct {
			ID           int64  `json:"id"`
			CustomerName string `json:"customerName"`
			Quantity     int    `json:"quantity"`
			Source       string `json:"source"`
			Status       string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(success.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != 0 || response.Data.ID != 1002 || response.Data.CustomerName != "accepted:Lane" ||
		response.Data.Quantity != 2 || response.Data.Source != "web" || response.Data.Status != "created" {
		t.Fatalf("success response=%s", success.Body.String())
	}
	originalService := service.Testservice()
	counter := &countingTestservice{ITestservice: originalService}
	service.RegisterTestservice(counter)
	t.Cleanup(func() { service.RegisterTestservice(originalService) })

	for _, test := range []struct {
		name string
		url  string
		body string
	}{
		{name: "missing query binding", url: "/hello/order", body: `{"customerName":"Lane","quantity":2}`},
		{name: "invalid validate length", url: "/hello/order?source=web", body: `{"customerName":"Li","quantity":2}`},
		{name: "invalid validate range", url: "/hello/order?source=web", body: `{"customerName":"Lane","quantity":101}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, test.url, bytes.NewBufferString(test.body))
			request.Header.Set("Content-Type", "application/json")
			server.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}

	pathFailure := httptest.NewRecorder()
	server.ServeHTTP(pathFailure, httptest.NewRequest(http.MethodGet, "/hello/order/-1", nil))
	if pathFailure.Code != http.StatusBadRequest {
		t.Fatalf("path validation status=%d body=%s", pathFailure.Code, pathFailure.Body.String())
	}
	var pathResponse struct {
		Details []struct {
			Field string `json:"field"`
			Tag   string `json:"tag"`
		} `json:"details"`
	}
	if err := json.Unmarshal(pathFailure.Body.Bytes(), &pathResponse); err != nil {
		t.Fatal(err)
	}
	if len(pathResponse.Details) != 1 || pathResponse.Details[0].Field != "ID" || pathResponse.Details[0].Tag != "gt" {
		t.Fatalf("path validation details=%s, want validate gt rule", pathFailure.Body.String())
	}
	if counter.createCalls != 0 {
		t.Fatalf("validation failures called Logic %d times", counter.createCalls)
	}
}

type countingTestservice struct {
	service.ITestservice
	createCalls int
}

func (service *countingTestservice) Create(ctx context.Context, input *model.TestModel) (*model.TestModel, error) {
	service.createCalls++
	return service.ITestservice.Create(ctx, input)
}

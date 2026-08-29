package router_test

import (
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lanechi/gonex/router"
)

type binderSnapshotRequest struct {
	Value string `query:"value"`
}

func TestBinderFieldsSnapshotDoesNotMutateRuntimePlan(t *testing.T) {
	gin.SetMode(gin.TestMode)
	binder, err := router.NewBinder(reflect.TypeOf((*binderSnapshotRequest)(nil)))
	if err != nil {
		t.Fatal(err)
	}
	if len(binder.Fields) != 1 {
		t.Fatalf("fields=%v", binder.Fields)
	}
	binder.Fields[0].Query = "tampered"
	binder.Fields = nil

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest("GET", "/?value=original&tampered=wrong", nil)
	var target binderSnapshotRequest
	if err := binder.Bind(context, &target); err != nil {
		t.Fatal(err)
	}
	if target.Value != "original" {
		t.Fatalf("value=%q, want original", target.Value)
	}
}

package ghttp

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDefaultErrorHandlerShowsDetailsOnlyInDebugMode(t *testing.T) {
	for _, test := range []struct {
		name          string
		mode          string
		expectDetails bool
	}{
		{name: "debug", mode: gin.DebugMode, expectDetails: true},
		{name: "release", mode: gin.ReleaseMode, expectDetails: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			writer := httptest.NewRecorder()
			ginContext, _ := gin.CreateTestContext(writer)
			server := &Server{mode: test.mode, modeSet: true}
			server.applyModeConfig()
			defaultErrorHandler(&Context{gin: ginContext, server: server}, &Error{
				Code:       40002,
				HTTPStatus: 400,
				Message:    "request validation failed",
				Details:    []map[string]string{{"field": "name", "tag": "required"}},
			})

			var response Response
			if err := json.Unmarshal(writer.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Code != 40002 || response.Message != "request validation failed" {
				t.Fatalf("response=%#v", response)
			}
			if (response.Details != nil) != test.expectDetails {
				t.Fatalf("details=%#v, want present=%v", response.Details, test.expectDetails)
			}
		})
	}
}

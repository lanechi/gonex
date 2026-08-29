package ghttp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCookieSetRejectsAfterHeadersWritten(t *testing.T) {
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := newContext(ginContext)

	ginContext.Status(http.StatusNoContent)
	ginContext.Writer.WriteHeaderNow()
	if err := ctx.Cookie().Set("session_id", "value", CookieOptions{Path: "/", HTTPOnly: true}); err == nil {
		t.Fatal("cookie write succeeded after response headers were committed")
	}
	if values := recorder.Header().Values("Set-Cookie"); len(values) != 0 {
		t.Fatalf("late cookie write changed response headers: %v", values)
	}
}

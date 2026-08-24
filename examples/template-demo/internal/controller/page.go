package controller

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/lanechi/gonex/g"
	"github.com/lanechi/gonex/ghttp"
)

type PageReq struct {
	g.Meta `path:"/page" method:"get" tags:"Template" summary:"Render a template page"`
	Name   string `query:"name"`
}

type PageRes struct{}

type PageController struct{}

func NewPage() *PageController {
	return &PageController{}
}

func (*PageController) Page(ctx context.Context, req *PageReq) (*PageRes, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "gonex"
	}

	requestContext := ghttp.FromContext(ctx)

	if err := requestContext.HTML(http.StatusOK, "index.html", g.Map{
		"Name":       name,
		"Message":    "This page is rendered by html/template.",
		"RenderedAt": time.Now().Format(time.RFC3339),
	}); err != nil {
		return nil, err
	}
	return nil, nil
}

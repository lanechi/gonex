package v1

import "github.com/lanechi/gonex/g"

type HelloReq struct {
	g.Meta `path:"/hello" method:"get" tags:"Hello" summary:"Say hello"`
	Name   string `query:"name"`
}

type HelloRes struct {
	Message string `json:"message"`
}

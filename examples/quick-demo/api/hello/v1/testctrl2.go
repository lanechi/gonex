package v1

import "github.com/lanechi/gonex/g"

type CreateTestctrl2Req struct {
	g.Meta `path:"/testctrl2" method:"post" tags:"Hello" summary:"Create testctrl2"`
}

type CreateTestctrl2Res struct{}

type UpdateTestctrl2Req struct {
	g.Meta `path:"/testctrl2/:id" method:"put" tags:"Hello" summary:"Update testctrl2"`
	ID     int64 `path:"id" binding:"required"`
}

type UpdateTestctrl2Res struct{}

type DeleteTestctrl2Req struct {
	g.Meta `path:"/testctrl2/:id" method:"delete" tags:"Hello" summary:"Delete testctrl2"`
	ID     int64 `path:"id" binding:"required"`
}

type DeleteTestctrl2Res struct{}

type GetOneTestctrl2Req struct {
	g.Meta `path:"/truetest/:id" method:"get" tags:"Hello" summary:"Get one testctrl2"`
	ID     int64 `path:"id" binding:"required,lte=5"  dc:"测试的"`
}

type GetOneTestctrl2Res struct{}

type GetListTestctrl2Req struct {
	g.Meta `path:"/testctrl2" method:"get" tags:"Hello" summary:"Get testctrl2 list"`
}

type GetListTestctrl2Res struct{}

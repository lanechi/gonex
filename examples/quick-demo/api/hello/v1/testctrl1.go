package v1

import "github.com/lanechi/gonex/g"

type CreateTestctrl1Req struct {
	g.Meta `path:"/testctrl1" method:"post" tags:"Hello" summary:"Create testctrl1"`
}

type CreateTestctrl1Res struct{}

type UpdateTestctrl1Req struct {
	g.Meta `path:"/testctrl1/:id" method:"put" tags:"Hello" summary:"Update testctrl1"`
	ID     int64 `path:"id" binding:"required"`
}

type UpdateTestctrl1Res struct{}

type DeleteTestctrl1Req struct {
	g.Meta `path:"/testctrl1/:id" method:"delete" tags:"Hello" summary:"Delete testctrl1"`
	ID     int64 `path:"id" binding:"required"`
}

type DeleteTestctrl1Res struct{}

type GetOneTestctrl1Req struct {
	g.Meta `path:"/testctrl1/:id" method:"get" tags:"Hello" summary:"Get one testctrl1"`
	ID     int64 `path:"id" binding:"required"`
}

type GetOneTestctrl1Res struct{}

type GetListTestctrl1Req struct {
	g.Meta `path:"/testctrl1" method:"get" tags:"Hello" summary:"Get testctrl1 list"`
}

type GetListTestctrl1Res struct{}

type GetListTestctrl12Req struct {
	g.Meta `path:"/testctrl12" method:"get" tags:"Hello" summary:"Get testctrl1 list"`
}

type GetListTestctrl12Res struct{}

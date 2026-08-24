package v1

import "github.com/lanechi/gonex/g"

type CreateUserReq struct {
	g.Meta `path:"/user" method:"post" tags:"Hello" summary:"Create user"`
}

type CreateUserRes struct{}

type UpdateUserReq struct {
	g.Meta `path:"/user/:id" method:"put" tags:"Hello" summary:"Update user"`
	ID     int64 `path:"id" binding:"required"`
}

type UpdateUserRes struct{}

type DeleteUserReq struct {
	g.Meta `path:"/user/:id" method:"delete" tags:"Hello" summary:"Delete user"`
	ID     int64 `path:"id" binding:"required"`
}

type DeleteUserRes struct{}

type GetOneUserReq struct {
	g.Meta `path:"/user/:id" method:"get" tags:"Hello" summary:"Get one user"`
	ID     int64 `path:"id" binding:"required"`
}

type GetOneUserRes struct{}

type GetListUserReq struct {
	g.Meta `path:"/user" method:"get" tags:"Hello" summary:"Get user list"`
}

type GetListUserRes struct{}

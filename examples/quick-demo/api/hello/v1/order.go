package v1

import "github.com/lanechi/gonex/g"

type CreateOrderReq struct {
	g.Meta       `path:"/order" method:"post" tags:"Hello" summary:"Create order"`
	CustomerName string `json:"customerName" binding:"required" validate:"min=3"`
	Quantity     int    `json:"quantity" binding:"required" validate:"gte=1,lte=100"`
	Source       string `query:"source" binding:"required" validate:"oneof=web mobile"`
}

type CreateOrderRes struct {
	ID           int64  `json:"id"`
	CustomerName string `json:"customerName"`
	Quantity     int    `json:"quantity"`
	Source       string `json:"source"`
	Status       string `json:"status"`
}

type UpdateOrderReq struct {
	g.Meta `path:"/order/:id" method:"put" tags:"Hello" summary:"Update order"`
	ID     int64 `path:"id" binding:"required" validate:"gt=0"`
}

type UpdateOrderRes struct{}

type DeleteOrderReq struct {
	g.Meta `path:"/order/:id" method:"delete" tags:"Hello" summary:"Delete order"`
	ID     int64 `path:"id" binding:"required" validate:"gt=0"`
}

type DeleteOrderRes struct{}

type GetOneOrderReq struct {
	g.Meta `path:"/order/:id" method:"get" tags:"Hello" summary:"Get one order"`
	ID     int64 `path:"id" binding:"required" validate:"gt=0"`
}

type GetOneOrderRes struct {
	ID int64 `json:"id"`
}

type GetListOrderReq struct {
	g.Meta `path:"/order" method:"get" tags:"Hello" summary:"Get order list"`
}

type GetListOrderRes struct{}

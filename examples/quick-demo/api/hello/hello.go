package hello

import (
	"context"

	v1 "github.com/lanechi/gonex/examples/quick-demo/api/hello/v1"
)

type IHelloV1 interface {
	CreateOrder(ctx context.Context, req *v1.CreateOrderReq) (*v1.CreateOrderRes, error)
	CreateTestctrl1(ctx context.Context, req *v1.CreateTestctrl1Req) (*v1.CreateTestctrl1Res, error)
	CreateTestctrl2(ctx context.Context, req *v1.CreateTestctrl2Req) (*v1.CreateTestctrl2Res, error)
	CreateUser(ctx context.Context, req *v1.CreateUserReq) (*v1.CreateUserRes, error)
	DeleteOrder(ctx context.Context, req *v1.DeleteOrderReq) (*v1.DeleteOrderRes, error)
	DeleteTestctrl1(ctx context.Context, req *v1.DeleteTestctrl1Req) (*v1.DeleteTestctrl1Res, error)
	DeleteTestctrl2(ctx context.Context, req *v1.DeleteTestctrl2Req) (*v1.DeleteTestctrl2Res, error)
	DeleteUser(ctx context.Context, req *v1.DeleteUserReq) (*v1.DeleteUserRes, error)
	GetListOrder(ctx context.Context, req *v1.GetListOrderReq) (*v1.GetListOrderRes, error)
	GetListTestctrl1(ctx context.Context, req *v1.GetListTestctrl1Req) (*v1.GetListTestctrl1Res, error)
	GetListTestctrl12(ctx context.Context, req *v1.GetListTestctrl12Req) (*v1.GetListTestctrl12Res, error)
	GetListTestctrl2(ctx context.Context, req *v1.GetListTestctrl2Req) (*v1.GetListTestctrl2Res, error)
	GetListUser(ctx context.Context, req *v1.GetListUserReq) (*v1.GetListUserRes, error)
	GetOneOrder(ctx context.Context, req *v1.GetOneOrderReq) (*v1.GetOneOrderRes, error)
	GetOneTestctrl1(ctx context.Context, req *v1.GetOneTestctrl1Req) (*v1.GetOneTestctrl1Res, error)
	GetOneTestctrl2(ctx context.Context, req *v1.GetOneTestctrl2Req) (*v1.GetOneTestctrl2Res, error)
	GetOneUser(ctx context.Context, req *v1.GetOneUserReq) (*v1.GetOneUserRes, error)
	UpdateOrder(ctx context.Context, req *v1.UpdateOrderReq) (*v1.UpdateOrderRes, error)
	UpdateTestctrl1(ctx context.Context, req *v1.UpdateTestctrl1Req) (*v1.UpdateTestctrl1Res, error)
	UpdateTestctrl2(ctx context.Context, req *v1.UpdateTestctrl2Req) (*v1.UpdateTestctrl2Res, error)
	UpdateUser(ctx context.Context, req *v1.UpdateUserReq) (*v1.UpdateUserRes, error)
}

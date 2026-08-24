package hello

import (
	"context"
	"github.com/lanechi/gonex/examples/quick-demo/api/hello/v1"
)

func (*ControllerV1) CreateUser(ctx context.Context, req *v1.CreateUserReq) (*v1.CreateUserRes, error) {
	return &v1.CreateUserRes{}, nil
}
func (*ControllerV1) DeleteUser(ctx context.Context, req *v1.DeleteUserReq) (*v1.DeleteUserRes, error) {
	return &v1.DeleteUserRes{}, nil
}
func (*ControllerV1) GetListUser(ctx context.Context, req *v1.GetListUserReq) (*v1.GetListUserRes, error) {
	return &v1.GetListUserRes{}, nil
}
func (*ControllerV1) GetOneUser(ctx context.Context, req *v1.GetOneUserReq) (*v1.GetOneUserRes, error) {
	return &v1.GetOneUserRes{}, nil
}
func (*ControllerV1) UpdateUser(ctx context.Context, req *v1.UpdateUserReq) (*v1.UpdateUserRes, error) {
	return &v1.UpdateUserRes{}, nil
}

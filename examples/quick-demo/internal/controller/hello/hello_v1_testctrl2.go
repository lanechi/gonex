package hello

import (
	"context"
	"github.com/lanechi/gonex/examples/quick-demo/api/hello/v1"
)

func (*ControllerV1) CreateTestctrl2(ctx context.Context, req *v1.CreateTestctrl2Req) (*v1.CreateTestctrl2Res, error) {
	return &v1.CreateTestctrl2Res{}, nil
}
func (*ControllerV1) DeleteTestctrl2(ctx context.Context, req *v1.DeleteTestctrl2Req) (*v1.DeleteTestctrl2Res, error) {
	return &v1.DeleteTestctrl2Res{}, nil
}
func (*ControllerV1) GetListTestctrl2(ctx context.Context, req *v1.GetListTestctrl2Req) (*v1.GetListTestctrl2Res, error) {
	return &v1.GetListTestctrl2Res{}, nil
}
func (*ControllerV1) GetOneTestctrl2(ctx context.Context, req *v1.GetOneTestctrl2Req) (*v1.GetOneTestctrl2Res, error) {
	return &v1.GetOneTestctrl2Res{}, nil
}
func (*ControllerV1) UpdateTestctrl2(ctx context.Context, req *v1.UpdateTestctrl2Req) (*v1.UpdateTestctrl2Res, error) {
	return &v1.UpdateTestctrl2Res{}, nil
}

package hello

import (
	"context"
	"github.com/lanechi/gonex/examples/quick-demo/api/hello/v1"
)

func (*ControllerV1) CreateTestctrl1(ctx context.Context, req *v1.CreateTestctrl1Req) (*v1.CreateTestctrl1Res, error) {
	return &v1.CreateTestctrl1Res{}, nil
}
func (*ControllerV1) DeleteTestctrl1(ctx context.Context, req *v1.DeleteTestctrl1Req) (*v1.DeleteTestctrl1Res, error) {
	return &v1.DeleteTestctrl1Res{}, nil
}
func (*ControllerV1) GetListTestctrl1(ctx context.Context, req *v1.GetListTestctrl1Req) (*v1.GetListTestctrl1Res, error) {
	return &v1.GetListTestctrl1Res{}, nil
}
func (*ControllerV1) GetListTestctrl12(ctx context.Context, req *v1.GetListTestctrl12Req) (*v1.GetListTestctrl12Res, error) {
	return &v1.GetListTestctrl12Res{}, nil
}
func (*ControllerV1) GetOneTestctrl1(ctx context.Context, req *v1.GetOneTestctrl1Req) (*v1.GetOneTestctrl1Res, error) {
	return &v1.GetOneTestctrl1Res{}, nil
}
func (*ControllerV1) UpdateTestctrl1(ctx context.Context, req *v1.UpdateTestctrl1Req) (*v1.UpdateTestctrl1Res, error) {
	return &v1.UpdateTestctrl1Res{}, nil
}

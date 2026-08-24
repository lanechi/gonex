package hello

import (
	"context"

	v1 "github.com/lanechi/gonex/examples/quick-demo/api/hello/v1"
	"github.com/lanechi/gonex/examples/quick-demo/internal/model"
	"github.com/lanechi/gonex/examples/quick-demo/internal/service"
)

func (*ControllerV1) CreateOrder(ctx context.Context, req *v1.CreateOrderReq) (*v1.CreateOrderRes, error) {
	created, err := service.Testservice().Create(ctx, &model.TestModel{
		ID:   int64(req.Quantity),
		Name: req.CustomerName,
	})
	if err != nil {
		return nil, err
	}
	return &v1.CreateOrderRes{
		ID:           created.ID,
		CustomerName: created.Name,
		Quantity:     req.Quantity,
		Source:       req.Source,
		Status:       "created",
	}, nil
}
func (*ControllerV1) DeleteOrder(ctx context.Context, req *v1.DeleteOrderReq) (*v1.DeleteOrderRes, error) {
	return &v1.DeleteOrderRes{}, nil
}
func (*ControllerV1) GetListOrder(ctx context.Context, req *v1.GetListOrderReq) (*v1.GetListOrderRes, error) {
	return &v1.GetListOrderRes{}, nil
}
func (*ControllerV1) GetOneOrder(ctx context.Context, req *v1.GetOneOrderReq) (*v1.GetOneOrderRes, error) {
	return &v1.GetOneOrderRes{ID: req.ID}, nil
}
func (*ControllerV1) UpdateOrder(ctx context.Context, req *v1.UpdateOrderReq) (*v1.UpdateOrderRes, error) {
	return &v1.UpdateOrderRes{}, nil
}

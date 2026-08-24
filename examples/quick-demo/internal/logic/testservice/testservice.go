package testservice

import (
	"context"
	"fmt"

	"github.com/lanechi/gonex/examples/quick-demo/internal/model"
	"github.com/lanechi/gonex/examples/quick-demo/internal/service"
)

type sTestservice struct{}

func init() {
	service.RegisterTestservice(New())
}

func New() service.ITestservice {
	return &sTestservice{}
}

func (*sTestservice) Create(ctx context.Context, input *model.TestModel) (*model.TestModel, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if input == nil {
		return nil, fmt.Errorf("create testservice order: input is nil")
	}
	return &model.TestModel{
		ID:   1000 + input.ID,
		Name: "accepted:" + input.Name,
	}, nil
}

func (*sTestservice) Update(_ context.Context, _ *model.TestModel) (*model.TestModel, error) {
	return &model.TestModel{}, nil
}

func (*sTestservice) Delete(_ context.Context, _ *model.TestModel) (*model.TestModel, error) {
	return &model.TestModel{}, nil
}

func (*sTestservice) GetOne(_ context.Context, _ *model.TestModel) (*model.TestModel, error) {
	return &model.TestModel{}, nil
}

func (*sTestservice) GetList(_ context.Context, _ *model.TestModel) (*model.TestModelList, error) {
	return &model.TestModelList{}, nil
}

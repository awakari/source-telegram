package storage

import (
	"context"
	"github.com/awakari/source-telegram/model"
)

type storageMock struct {
}

func NewStorageMock() Storage {
	return storageMock{}
}

func (s storageMock) Close() error {
	return nil
}

func (s storageMock) Update(ctx context.Context, ch model.Channel) (err error) {
	//TODO implement me
	panic("implement me")
}

func (s storageMock) GetPage(ctx context.Context, filter model.ChannelFilter, limit uint32, cursor string) (page []model.Channel, err error) {
	switch cursor {
	case "":
		page = []model.Channel{
			{
				Id:   -1001801930101,
				Name: "channel0",
			},
			{
				Id:   -1001754252633,
				Name: "channel1",
			},
		}
	}
	return
}

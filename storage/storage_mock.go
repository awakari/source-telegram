package storage

import (
	"context"
	"github.com/awakari/source-telegram/model"
	"math"
)

type storageMock struct {
}

func NewStorageMock() Storage {
	return storageMock{}
}

func (s storageMock) Close() error {
	return nil
}

func (s storageMock) Exists(ctx context.Context, id int64) (exists bool, err error) {
	//TODO implement me
	panic("implement me")
}

func (s storageMock) GetPage(ctx context.Context, filter model.ChannelFilter, limit uint32, cursor int64) (page []int64, err error) {
	switch cursor {
	case math.MinInt64:
		page = []int64{
			-1001801930101,
			-1001754252633,
		}
	}
	return
}

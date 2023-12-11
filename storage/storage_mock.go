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

func (s storageMock) Create(ctx context.Context, ch model.Channel) (err error) {
	switch ch.Name {
	case "fail":
		err = ErrInternal
	case "conflict":
		err = ErrConflict
	}
	return
}

func (s storageMock) Read(ctx context.Context, link string) (ch model.Channel, err error) {
	switch link {
	case "fail":
		err = ErrInternal
	case "missing":
		err = ErrNotFound
	default:
		ch.Id = -1001801930101
		ch.GroupId = "group0"
		ch.UserId = "user0"
		ch.Name = "channel0"
		ch.Link = "https://t.me/channel0"
	}
	return
}

func (s storageMock) Delete(ctx context.Context, link string) (err error) {
	switch link {
	case "fail":
		err = ErrInternal
	case "missing":
		err = ErrNotFound
	}
	return
}

func (s storageMock) GetPage(ctx context.Context, filter model.ChannelFilter, limit uint32, cursor string, order model.Order) (page []model.Channel, err error) {
	switch cursor {
	case "":
		switch order {
		case model.OrderDesc:
			page = []model.Channel{
				{
					Name: "channel1",
					Link: "https://t.me/c/1/2",
				},
				{
					Name: "channel0",
					Link: "https://t.me/channel0",
				},
			}
		default:
			page = []model.Channel{
				{
					Name: "channel0",
					Link: "https://t.me/channel0",
				},
				{
					Name: "channel1",
					Link: "https://t.me/c/1/2",
				},
			}
		}
	}
	return
}

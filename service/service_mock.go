package service

import (
	"context"
	"github.com/awakari/source-telegram/model"
	"github.com/awakari/source-telegram/storage"
)

type serviceMock struct {
}

func NewServiceMock() Service {
	return serviceMock{}
}

func (s serviceMock) Create(ctx context.Context, ch model.Channel) (err error) {
	switch ch.Name {
	case "fail":
		err = storage.ErrInternal
	case "conflict":
		err = storage.ErrConflict
	}
	return
}

func (s serviceMock) Read(ctx context.Context, link string) (ch model.Channel, err error) {
	switch link {
	case "fail":
		err = storage.ErrInternal
	case "missing":
		err = storage.ErrNotFound
	default:
		ch.Id = -1001801930101
		ch.GroupId = "group0"
		ch.UserId = "user0"
		ch.Name = "channel0"
		ch.Link = "https://t.me/channel0"
	}
	return
}

func (s serviceMock) Delete(ctx context.Context, link string) (err error) {
	switch link {
	case "fail":
		err = storage.ErrInternal
	case "missing":
		err = storage.ErrNotFound
	}
	return
}

func (s serviceMock) GetPage(ctx context.Context, filter model.ChannelFilter, limit uint32, cursor string) (page []model.Channel, err error) {
	switch cursor {
	case "":
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
	return
}

func (s serviceMock) RefreshJoinedLoop() (err error) {
	//TODO implement me
	panic("implement me")
}

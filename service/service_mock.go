package service

import (
	"context"
	"errors"
	"github.com/awakari/source-telegram/model"
	"github.com/awakari/source-telegram/storage"
	"github.com/cloudevents/sdk-go/binding/format/protobuf/v2/pb"
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
	case "nobot":
		err = ErrNoBot
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
		ch.Label = "1"
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

func (s serviceMock) GetPage(ctx context.Context, filter model.ChannelFilter, limit uint32, cursor string, order model.Order) (page []model.Channel, err error) {
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

func (s serviceMock) SearchAndAdd(ctx context.Context, groupId, subId, terms string, limit uint32) (n uint32, err error) {
	switch terms {
	case "fail":
		err = errors.New("fail")
	default:
		n = 42
	}
	return
}

func (s serviceMock) RefreshJoinedLoop() (err error) {
	//TODO implement me
	panic("implement me")
}

func (s serviceMock) HandleInterestChange(ctx context.Context, evt *pb.CloudEvent) (err error) {
	//TODO implement me
	panic("implement me")
}

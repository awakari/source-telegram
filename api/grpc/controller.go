package grpc

import (
	"context"
	"errors"
	"github.com/awakari/source-telegram/model"
	"github.com/awakari/source-telegram/service"
	"github.com/awakari/source-telegram/storage"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"
)

type controller struct {
	svc service.Service
}

func NewController(svc service.Service) ServiceServer {
	return controller{
		svc: svc,
	}
}

func (c controller) Create(ctx context.Context, req *CreateRequest) (resp *CreateResponse, err error) {
	resp = &CreateResponse{}
	if req.Channel == nil {
		err = status.Error(codes.InvalidArgument, "channel payload is missing")
	}
	if err == nil {
		ch := model.Channel{
			Id:      req.Channel.Id,
			GroupId: req.Channel.GroupId,
			UserId:  req.Channel.UserId,
			Name:    req.Channel.Name,
			Link:    req.Channel.Link,
			Created: time.Now().UTC(),
		}
		err = c.svc.Create(ctx, ch)
		err = encodeError(err)
	}
	return
}

func (c controller) Read(ctx context.Context, req *ReadRequest) (resp *ReadResponse, err error) {
	resp = &ReadResponse{}
	var ch model.Channel
	ch, err = c.svc.Read(ctx, req.Link)
	switch err {
	case nil:
		resp.Channel = &Channel{
			Id:      ch.Id,
			GroupId: ch.GroupId,
			UserId:  ch.UserId,
			Name:    ch.Name,
			Link:    ch.Link,
			SubId:   ch.SubId,
			Terms:   ch.Terms,
		}
		if !ch.Created.IsZero() {
			resp.Channel.Created = timestamppb.New(ch.Created)
		}
		if !ch.Last.IsZero() {
			resp.Channel.Last = timestamppb.New(ch.Last)
		}
	default:
		err = encodeError(err)
	}
	return
}

func (c controller) Delete(ctx context.Context, req *DeleteRequest) (resp *DeleteResponse, err error) {
	resp = &DeleteResponse{}
	err = c.svc.Delete(ctx, req.Link)
	err = encodeError(err)
	return
}

func (c controller) List(ctx context.Context, req *ListRequest) (resp *ListResponse, err error) {
	resp = &ListResponse{}
	filter := model.ChannelFilter{}
	if req.Filter != nil {
		filter.GroupId = req.Filter.GroupId
		filter.UserId = req.Filter.UserId
		filter.Pattern = req.Filter.Pattern
	}
	var order model.Order
	switch req.Order {
	case Order_DESC:
		order = model.OrderDesc
	default:
		order = model.OrderAsc
	}
	var page []model.Channel
	page, err = c.svc.GetPage(ctx, filter, req.Limit, req.Cursor, order)
	if len(page) > 0 {
		for _, ch := range page {
			resp.Page = append(resp.Page, &Channel{
				Id:      ch.Id,
				GroupId: ch.GroupId,
				UserId:  ch.UserId,
				Name:    ch.Name,
				Link:    ch.Link,
			})
		}
	}
	err = encodeError(err)
	return
}

func (c controller) SearchAndAdd(ctx context.Context, req *SearchAndAddRequest) (resp *SearchAndAddResponse, err error) {
	resp = &SearchAndAddResponse{}
	resp.CountAdded, err = c.svc.SearchAndAdd(ctx, req.GroupId, req.SubId, req.Terms, req.Limit)
	err = encodeError(err)
	return
}

func encodeError(src error) (dst error) {
	switch {
	case src == nil:
	case errors.Is(src, storage.ErrConflict):
		dst = status.Error(codes.AlreadyExists, src.Error())
	case errors.Is(src, storage.ErrNotFound):
		dst = status.Error(codes.NotFound, src.Error())
	case errors.Is(src, storage.ErrInternal):
		dst = status.Error(codes.Internal, src.Error())
	case errors.Is(src, context.DeadlineExceeded):
		dst = status.Error(codes.DeadlineExceeded, src.Error())
	case errors.Is(src, context.Canceled):
		dst = status.Error(codes.Canceled, src.Error())
	default:
		dst = status.Error(codes.Unknown, src.Error())
	}
	return
}

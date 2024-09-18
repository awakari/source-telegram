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

type Controller interface {
	SetService(svc service.Service)
	ServiceServer
}

type controller struct {
	svc    service.Service
	chCode chan string
}

func NewController(chCode chan string) Controller {
	return &controller{
		chCode: chCode,
	}
}

func (c *controller) SetService(svc service.Service) {
	c.svc = svc
	return
}

func (c *controller) Create(ctx context.Context, req *CreateRequest) (resp *CreateResponse, err error) {
	resp = &CreateResponse{}
	if c.svc == nil {
		err = status.Error(codes.FailedPrecondition, "service not initialized")
	}
	if err == nil && req.Channel == nil {
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
			Label:   req.Channel.Label,
		}
		err = c.svc.Create(ctx, ch)
		err = encodeError(err)
	}
	return
}

func (c *controller) Read(ctx context.Context, req *ReadRequest) (resp *ReadResponse, err error) {
	resp = &ReadResponse{}
	if c.svc == nil {
		err = status.Error(codes.FailedPrecondition, "service not initialized")
	}
	var ch model.Channel
	if err == nil {
		ch, err = c.svc.Read(ctx, req.Link)
	}
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
			Label:   ch.Label,
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

func (c *controller) Delete(ctx context.Context, req *DeleteRequest) (resp *DeleteResponse, err error) {
	resp = &DeleteResponse{}
	if c.svc == nil {
		err = status.Error(codes.FailedPrecondition, "service not initialized")
	}
	if err == nil {
		err = c.svc.Delete(ctx, req.Link)
		err = encodeError(err)
	}
	return
}

func (c *controller) List(ctx context.Context, req *ListRequest) (resp *ListResponse, err error) {
	resp = &ListResponse{}
	if c.svc == nil {
		err = status.Error(codes.FailedPrecondition, "service not initialized")
	}
	var page []model.Channel
	if err == nil {
		filter := model.ChannelFilter{}
		if req.Filter != nil {
			filter.GroupId = req.Filter.GroupId
			filter.UserId = req.Filter.UserId
			filter.Pattern = req.Filter.Pattern
			filter.SubId = req.Filter.SubId
		}
		var order model.Order
		switch req.Order {
		case Order_DESC:
			order = model.OrderDesc
		default:
			order = model.OrderAsc
		}
		page, err = c.svc.GetPage(ctx, filter, req.Limit, req.Cursor, order)
	}
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

func (c *controller) SearchAndAdd(ctx context.Context, req *SearchAndAddRequest) (resp *SearchAndAddResponse, err error) {
	resp = &SearchAndAddResponse{}
	if c.svc == nil {
		err = status.Error(codes.FailedPrecondition, "service not initialized")
	}
	if err == nil {
		resp.CountAdded, err = c.svc.SearchAndAdd(ctx, req.GroupId, req.SubId, req.Terms, req.Limit)
		err = encodeError(err)
	}
	return
}

func (c *controller) Login(ctx context.Context, req *LoginRequest) (resp *LoginResponse, err error) {
	resp = &LoginResponse{}
	select {
	case c.chCode <- req.Code:
		resp.Success = true
	default:
		resp.Success = false // doesn't accept anymore
	}
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
	case errors.Is(src, service.ErrNoBot):
		dst = status.Error(codes.PermissionDenied, src.Error())
	case errors.Is(src, context.DeadlineExceeded):
		dst = status.Error(codes.DeadlineExceeded, src.Error())
	case errors.Is(src, context.Canceled):
		dst = status.Error(codes.Canceled, src.Error())
	case status.Code(src) == codes.FailedPrecondition:
		dst = src
	default:
		dst = status.Error(codes.Unknown, src.Error())
	}
	return
}

package grpc

import (
	"context"
	"errors"
	"github.com/awakari/source-telegram/model"
	"github.com/awakari/source-telegram/storage"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type controller struct {
	stor storage.Storage
}

func NewController(stor storage.Storage) ServiceServer {
	return controller{
		stor: stor,
	}
}

func (c controller) List(ctx context.Context, req *ListRequest) (resp *ListResponse, err error) {
	filterNone := model.ChannelFilter{}
	resp = &ListResponse{}
	resp.Page, err = c.stor.GetPage(ctx, filterNone, req.Limit, req.Cursor)
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

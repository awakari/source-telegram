package storage

import (
	"context"
	"errors"
	"github.com/awakari/source-telegram/model"
	"io"
)

type Storage interface {
	io.Closer
	Update(ctx context.Context, ch model.Channel) (err error)
	GetPage(ctx context.Context, filter model.ChannelFilter, limit uint32, cursor string) (page []model.Channel, err error)
}

var ErrNotFound = errors.New("channel not found")
var ErrInternal = errors.New("internal failure")
var ErrConflict = errors.New("channel with the same id is already present and not expired")

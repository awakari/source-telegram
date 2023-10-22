package storage

import (
	"context"
	"errors"
	"github.com/awakari/source-telegram/model"
	"io"
)

type Storage interface {
	io.Closer
	Exists(ctx context.Context, id int64) (exists bool, err error)
	GetPage(ctx context.Context, filter model.ChannelFilter, limit uint32, cursor int64) (page []int64, err error)
}

var ErrNotFound = errors.New("channel not found")
var ErrInternal = errors.New("internal failure")
var ErrConflict = errors.New("channel with the same id is already present and not expired")

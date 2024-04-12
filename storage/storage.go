package storage

import (
    "context"
    "errors"
    "github.com/awakari/source-telegram/model"
    "io"
    "time"
)

type Storage interface {
    io.Closer
    Create(ctx context.Context, ch model.Channel) (err error)
    Read(ctx context.Context, link string) (ch model.Channel, err error)
    Update(ctx context.Context, link string, last time.Time) (err error)
    Delete(ctx context.Context, link string) (err error)
    GetPage(ctx context.Context, filter model.ChannelFilter, limit uint32, cursor string, order model.Order) (page []model.Channel, err error)
}

var ErrNotFound = errors.New("channel not found")
var ErrInternal = errors.New("internal failure")
var ErrConflict = errors.New("channel with the same id is already present")

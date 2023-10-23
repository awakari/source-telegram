package storage

import (
	"context"
	"fmt"
	"github.com/awakari/source-telegram/model"
	"log/slog"
)

type storageLogging struct {
	stor Storage
	log  *slog.Logger
}

func NewStorageLogging(stor Storage, log *slog.Logger) Storage {
	return storageLogging{
		stor: stor,
		log:  log,
	}
}

func (sl storageLogging) Close() (err error) {
	err = sl.stor.Close()
	ll := sl.logLevel(err)
	sl.log.Log(context.TODO(), ll, fmt.Sprintf("storage.Close(): %s", err))
	return
}

func (sl storageLogging) Update(ctx context.Context, ch model.Channel) (err error) {
	err = sl.stor.Update(ctx, ch)
	ll := sl.logLevel(err)
	sl.log.Log(ctx, ll, fmt.Sprintf("storage.Update(%+v): %s", ch, err))
	return
}

func (sl storageLogging) GetPage(ctx context.Context, filter model.ChannelFilter, limit uint32, cursor string) (page []model.Channel, err error) {
	page, err = sl.stor.GetPage(ctx, filter, limit, cursor)
	ll := sl.logLevel(err)
	sl.log.Log(ctx, ll, fmt.Sprintf("storage.GetPage(filter=%+v, limit=%d, cursor=%s): %d, %s", filter, limit, cursor, len(page), err))
	return
}

func (sl storageLogging) logLevel(err error) (lvl slog.Level) {
	switch err {
	case nil:
		lvl = slog.LevelDebug
	default:
		lvl = slog.LevelError
	}
	return
}

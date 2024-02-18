package storage

import (
	"context"
	"github.com/awakari/source-telegram/model"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"time"
)

type localCache struct {
	stor  Storage
	cache *expirable.LRU[string, model.Channel]
}

func NewLocalCache(stor Storage, size int, ttl time.Duration) Storage {
	c := expirable.NewLRU[string, model.Channel](size, nil, ttl)
	return localCache{
		stor:  stor,
		cache: c,
	}
}

func (lc localCache) Close() error {
	lc.cache.Purge()
	return lc.stor.Close()
}

func (lc localCache) Create(ctx context.Context, ch model.Channel) (err error) {
	err = lc.stor.Create(ctx, ch)
	if err != nil {
		lc.cache.Add(ch.Link, ch)
	}
	return
}

func (lc localCache) Read(ctx context.Context, link string) (ch model.Channel, err error) {
	var found bool
	ch, found = lc.cache.Get(link)
	if !found {
		ch, err = lc.stor.Read(ctx, link)
	}
	return
}

func (lc localCache) Delete(ctx context.Context, link string) (err error) {
	err = lc.stor.Delete(ctx, link)
	if err != nil {
		lc.cache.Remove(link)
	}
	return
}

func (lc localCache) GetPage(ctx context.Context, filter model.ChannelFilter, limit uint32, cursor string, order model.Order) (page []model.Channel, err error) {
	page, err = lc.stor.GetPage(ctx, filter, limit, cursor, order)
	return
}

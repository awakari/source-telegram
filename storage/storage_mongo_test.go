package storage

import (
	"context"
	"fmt"
	"github.com/awakari/source-telegram/config"
	"github.com/awakari/source-telegram/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"os"
	"strconv"
	"testing"
	"time"
)

var dbUri = os.Getenv("DB_URI_TEST_MONGO")

func TestNewStorage(t *testing.T) {
	//
	collName := fmt.Sprintf("tgchans-test-%d", time.Now().UnixMicro())
	dbCfg := config.DbConfig{
		Uri:  dbUri,
		Name: "sources",
	}
	dbCfg.Table.Name = collName
	dbCfg.Table.Shard = false
	dbCfg.Tls.Enabled = true
	dbCfg.Tls.Insecure = true
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()
	s, err := NewStorage(ctx, dbCfg)
	assert.Nil(t, err)
	assert.NotNil(t, s)
	//
	clear(ctx, t, s.(storageMongo))
}

func clear(ctx context.Context, t *testing.T, s storageMongo) {
	require.Nil(t, s.coll.Drop(ctx))
	require.Nil(t, s.Close())
}

func TestStorageMongo_Create(t *testing.T) {
	//
	collName := fmt.Sprintf("feeds-test-%d", time.Now().UnixMicro())
	dbCfg := config.DbConfig{
		Uri:  dbUri,
		Name: "sources",
	}
	dbCfg.Table.Name = collName
	dbCfg.Tls.Enabled = true
	dbCfg.Tls.Insecure = true
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Hour)
	defer cancel()
	s, err := NewStorage(ctx, dbCfg)
	require.Nil(t, err)
	assert.NotNil(t, s)
	//
	sm := s.(storageMongo)
	defer clear(ctx, t, s.(storageMongo))
	//
	_, err = sm.coll.InsertOne(ctx, bson.M{
		attrId:   -1001801930101,
		attrLink: "https://t.me/chan0",
	})
	require.Nil(t, err)
	//
	cases := map[string]struct {
		in  model.Channel
		err error
	}{
		"ok": {
			in: model.Channel{
				Id:   -1001801930102,
				Link: "https://t.me/chan1",
			},
		},
		"dup id": {
			in: model.Channel{
				Id:   -1001801930101,
				Link: "https://t.me/chan1",
			},
			err: ErrConflict,
		},
		"dup link": {
			in: model.Channel{
				Id:   -1001801930102,
				Link: "https://t.me/chan0",
			},
			err: ErrConflict,
		},
	}
	//
	for k, c := range cases {
		t.Run(k, func(t *testing.T) {
			err = s.Create(ctx, c.in)
			assert.ErrorIs(t, err, c.err)
		})
	}
}

func TestStorageMongo_Read(t *testing.T) {
	//
	collName := fmt.Sprintf("feeds-test-%d", time.Now().UnixMicro())
	dbCfg := config.DbConfig{
		Uri:  dbUri,
		Name: "sources",
	}
	dbCfg.Table.Name = collName
	dbCfg.Tls.Enabled = true
	dbCfg.Tls.Insecure = true
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Hour)
	defer cancel()
	s, err := NewStorage(ctx, dbCfg)
	require.Nil(t, err)
	assert.NotNil(t, s)
	//
	sm := s.(storageMongo)
	defer clear(ctx, t, s.(storageMongo))
	//
	_, err = sm.coll.InsertOne(ctx, bson.M{
		attrId:      -1001801930101,
		attrGroupId: "group0",
		attrUserId:  "user0",
		attrName:    "channel 0",
		attrLink:    "https://t.me/chan0",
	})
	require.Nil(t, err)
	//
	cases := map[string]struct {
		link string
		out  model.Channel
		err  error
	}{
		"ok": {
			link: "https://t.me/chan0",
			out: model.Channel{
				Id:      -1001801930101,
				GroupId: "group0",
				UserId:  "user0",
				Name:    "channel 0",
				Link:    "https://t.me/chan0",
			},
		},
		"dup id": {
			link: "https://t.me/chan1",
			err:  ErrNotFound,
		},
	}
	//
	for k, c := range cases {
		t.Run(k, func(t *testing.T) {
			var ch model.Channel
			ch, err = s.Read(ctx, c.link)
			assert.Equal(t, c.out, ch)
			assert.ErrorIs(t, err, c.err)
		})
	}
}

func TestStorageMongo_Delete(t *testing.T) {
	//
	collName := fmt.Sprintf("feeds-test-%d", time.Now().UnixMicro())
	dbCfg := config.DbConfig{
		Uri:  dbUri,
		Name: "sources",
	}
	dbCfg.Table.Name = collName
	dbCfg.Tls.Enabled = true
	dbCfg.Tls.Insecure = true
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Hour)
	defer cancel()
	s, err := NewStorage(ctx, dbCfg)
	require.Nil(t, err)
	assert.NotNil(t, s)
	//
	sm := s.(storageMongo)
	defer clear(ctx, t, s.(storageMongo))
	//
	_, err = sm.coll.InsertOne(ctx, bson.M{
		attrId:   -1001801930101,
		attrLink: "https://t.me/chan0",
	})
	require.Nil(t, err)
	//
	cases := map[string]struct {
		link string
		err  error
	}{
		"ok": {
			link: "https://t.me/chan0",
		},
		"dup id": {
			link: "https://t.me/chan1",
			err:  ErrNotFound,
		},
	}
	//
	for k, c := range cases {
		t.Run(k, func(t *testing.T) {
			err = s.Delete(ctx, c.link)
			assert.ErrorIs(t, err, c.err)
		})
	}
}

func TestStorageMongo_Update(t *testing.T) {
	//
	collName := fmt.Sprintf("feeds-test-%d", time.Now().UnixMicro())
	dbCfg := config.DbConfig{
		Uri:  dbUri,
		Name: "sources",
	}
	dbCfg.Table.Name = collName
	dbCfg.Tls.Enabled = true
	dbCfg.Tls.Insecure = true
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Hour)
	defer cancel()
	s, err := NewStorage(ctx, dbCfg)
	require.Nil(t, err)
	assert.NotNil(t, s)
	//
	sm := s.(storageMongo)
	defer clear(ctx, t, s.(storageMongo))
	//
	_, err = sm.coll.InsertOne(ctx, bson.M{
		attrId:   -1001801930101,
		attrLink: "https://t.me/chan0",
	})
	require.Nil(t, err)
	//
	cases := map[string]struct {
		link string
		err  error
	}{
		"ok": {
			link: "https://t.me/chan0",
		},
		"missing": {
			link: "https://t.me/chan1",
			err:  ErrNotFound,
		},
	}
	//
	for k, c := range cases {
		t.Run(k, func(t *testing.T) {
			err = s.Update(ctx, c.link, time.Now())
			assert.ErrorIs(t, err, c.err)
		})
	}
}

func TestStorageMongo_GetPage(t *testing.T) {
	//
	collName := fmt.Sprintf("tgchans-test-%d", time.Now().UnixMicro())
	dbCfg := config.DbConfig{
		Uri:  dbUri,
		Name: "sources",
	}
	dbCfg.Table.Name = collName
	dbCfg.Tls.Enabled = true
	dbCfg.Tls.Insecure = true
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Hour)
	defer cancel()
	s, err := NewStorage(ctx, dbCfg)
	require.Nil(t, err)
	assert.NotNil(t, s)
	//
	sm := s.(storageMongo)
	defer clear(ctx, t, s.(storageMongo))
	//
	ids := []int64{
		-1001004621278,
		-1001024068640,
		-1001385274097,
		-1001754252633,
		-1001801930101,
	}
	for i, id := range ids {
		_, err = sm.coll.InsertOne(ctx, bson.M{
			attrId:    id,
			attrName:  strconv.FormatInt(id, 10),
			attrLink:  fmt.Sprintf("https://t.me/c/%s/123", strconv.FormatInt(-id, 10)),
			attrSubId: strconv.FormatInt(-id, 10),
			attrLabel: strconv.Itoa(i % 2),
		})
		require.Nil(t, err)
	}
	//
	cases := map[string]struct {
		filter model.ChannelFilter
		limit  uint32
		cursor string
		order  model.Order
		page   []int64
		err    error
	}{
		"basic": {
			limit: 10,
			page:  ids,
		},
		"filter w/ pattern": {
			filter: model.ChannelFilter{
				Pattern: "7",
			},
			limit: 10,
			page: []int64{
				ids[0],
				ids[2],
				ids[3],
			},
		},
		"filter": {
			filter: model.ChannelFilter{
				Label: "1",
			},
			limit: 10,
			page: []int64{
				ids[1],
				ids[3],
			},
		},
		"limit": {
			limit: 2,
			page: []int64{
				ids[0],
				ids[1],
			},
		},
		"cursor asc": {
			limit:  10,
			cursor: fmt.Sprintf("https://t.me/c/%s/123", strconv.FormatInt(-ids[1], 10)),
			page: []int64{
				ids[2],
				ids[3],
				ids[4],
			},
		},
		"cursor desc limit = 2": {
			limit:  2,
			cursor: fmt.Sprintf("https://t.me/c/%s/123", strconv.FormatInt(-ids[3], 10)),
			order:  model.OrderDesc,
			page: []int64{
				ids[2],
				ids[1],
			},
		},
		"end of results": {
			limit:  10,
			cursor: fmt.Sprintf("https://t.me/c/%s/123", strconv.FormatInt(-ids[4], 10)),
		},
		"filter by sub": {
			filter: model.ChannelFilter{
				SubId: "1001385274097",
			},
			limit: 10,
			page: []int64{
				ids[2],
			},
		},
	}
	//
	for k, c := range cases {
		t.Run(k, func(t *testing.T) {
			var page []model.Channel
			page, err = s.GetPage(ctx, c.filter, c.limit, c.cursor, c.order)
			assert.Equal(t, len(c.page), len(page))
			for i, ch := range page {
				assert.Equal(t, strconv.FormatInt(c.page[i], 10), ch.Name)
				assert.Equal(t, fmt.Sprintf("https://t.me/c/%d/123", -c.page[i]), ch.Link)
			}
			assert.ErrorIs(t, err, c.err)
		})
	}
}

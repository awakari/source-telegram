package storage

import (
	"context"
	"fmt"
	"github.com/awakari/source-telegram/config"
	"github.com/awakari/source-telegram/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"math"
	"os"
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

func TestStorageMongo_Exists(t *testing.T) {
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
		attrId: -1001801930101,
	})
	require.Nil(t, err)
	//
	var exists bool
	exists, err = s.Exists(ctx, -1001801930101)
	assert.True(t, exists)
	assert.Nil(t, err)
	exists, err = s.Exists(ctx, -1001024068640)
	assert.False(t, exists)
	assert.Nil(t, err)
}

func TestStorageMongo_GetPage(t *testing.T) {
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
	ids := []int64{
		-1001801930101,
		-1001754252633,
		-1001385274097,
		-1001024068640,
		-1001004621278,
	}
	for _, id := range ids {
		_, err = sm.coll.InsertOne(ctx, bson.M{
			attrId: id,
		})
		require.Nil(t, err)
	}
	//
	cases := map[string]struct {
		filter model.ChannelFilter
		limit  uint32
		cursor int64
		page   []int64
		err    error
	}{
		"basic": {
			limit:  10,
			cursor: math.MinInt64,
			page:   ids,
		},
		"filter": {
			filter: model.ChannelFilter{
				IdDiv: 2,
				IdRem: 1,
			},
			limit:  10,
			cursor: math.MinInt64,
			page: []int64{
				ids[0],
				ids[1],
				ids[2],
			},
		},
		"limit": {
			limit:  2,
			cursor: math.MinInt64,
			page: []int64{
				ids[0],
				ids[1],
			},
		},
		"cursor": {
			limit:  10,
			cursor: ids[1],
			page: []int64{
				ids[2],
				ids[3],
				ids[4],
			},
		},
		"end of results": {
			limit:  10,
			cursor: ids[4],
		},
	}
	//
	for k, c := range cases {
		t.Run(k, func(t *testing.T) {
			var page []int64
			page, err = s.GetPage(ctx, c.filter, c.limit, c.cursor)
			assert.Equal(t, c.page, page)
			assert.ErrorIs(t, err, c.err)
		})
	}
}

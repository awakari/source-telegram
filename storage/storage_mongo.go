package storage

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/awakari/source-telegram/config"
	"github.com/awakari/source-telegram/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type recChan struct {
	Id   int64  `bson:"id"`
	Name string `bson:"name"`
}

const attrId = "id"
const attrName = "name"

type storageMongo struct {
	conn *mongo.Client
	db   *mongo.Database
	coll *mongo.Collection
}

var optsSrvApi = options.ServerAPI(options.ServerAPIVersion1)
var optsGet = options.
	FindOne().
	SetShowRecordID(false)
var projGetBatch = bson.D{
	{
		Key:   attrId,
		Value: 1,
	},
	{
		Key:   attrName,
		Value: 1,
	},
}
var sortGetBatch = bson.D{
	{
		Key:   attrName,
		Value: 1,
	},
}
var indices = []mongo.IndexModel{
	{
		Keys: bson.D{
			{
				Key:   attrId,
				Value: 1,
			},
		},
		Options: options.
			Index().
			SetUnique(true),
	},
	{
		Keys: bson.D{
			{
				Key:   attrName,
				Value: 1,
			},
		},
		Options: options.
			Index().
			SetUnique(false),
	},
}

func NewStorage(ctx context.Context, cfgDb config.DbConfig) (s Storage, err error) {
	clientOpts := options.
		Client().
		ApplyURI(cfgDb.Uri).
		SetServerAPIOptions(optsSrvApi)
	if cfgDb.Tls.Enabled {
		clientOpts = clientOpts.SetTLSConfig(&tls.Config{InsecureSkipVerify: cfgDb.Tls.Insecure})
	}
	if len(cfgDb.UserName) > 0 {
		auth := options.Credential{
			Username:    cfgDb.UserName,
			Password:    cfgDb.Password,
			PasswordSet: len(cfgDb.Password) > 0,
		}
		clientOpts = clientOpts.SetAuth(auth)
	}
	conn, err := mongo.Connect(ctx, clientOpts)
	var sm storageMongo
	if err == nil {
		db := conn.Database(cfgDb.Name)
		coll := db.Collection(cfgDb.Table.Name)
		sm.conn = conn
		sm.db = db
		sm.coll = coll
		_, err = sm.ensureIndices(ctx)
	}
	if err == nil {
		s = sm
	}
	return
}

func (sm storageMongo) ensureIndices(ctx context.Context) ([]string, error) {
	return sm.coll.Indexes().CreateMany(ctx, indices)
}

func (sm storageMongo) Close() error {
	return sm.conn.Disconnect(context.TODO())
}

func (sm storageMongo) Update(ctx context.Context, ch model.Channel) (err error) {
	q := bson.M{
		attrId: ch.Id,
	}
	u := bson.M{
		"$set": bson.M{
			attrName: ch.Name,
		},
	}
	var result *mongo.UpdateResult
	result, err = sm.coll.UpdateOne(ctx, q, u)
	switch err {
	case nil:
		if result.MatchedCount < 1 {
			err = fmt.Errorf("%w: %d", ErrNotFound, ch.Id)
		}
	default:
		err = decodeError(err, ch.Id)
	}
	return
}

func (sm storageMongo) GetPage(ctx context.Context, filter model.ChannelFilter, limit uint32, cursor string) (page []model.Channel, err error) {
	q := bson.M{
		attrName: bson.M{
			"$gt": cursor,
		},
	}
	if filter.IdDiv != 0 {
		q[attrId] = bson.M{
			"$mod": bson.A{
				filter.IdDiv,
				-int32(filter.IdRem),
			},
		}
	}
	optsList := options.
		Find().
		SetLimit(int64(limit)).
		SetShowRecordID(false).
		SetSort(sortGetBatch).
		SetProjection(projGetBatch)
	var cur *mongo.Cursor
	cur, err = sm.coll.Find(ctx, q, optsList)
	if err == nil {
		var rec recChan
		for cur.Next(ctx) {
			err = errors.Join(err, cur.Decode(&rec))
			if err == nil {
				page = append(page, model.Channel{
					Id:   rec.Id,
					Name: rec.Name,
				})
			}
		}
	}
	err = decodeError(err, 0)
	return
}

func decodeError(src error, id int64) (dst error) {
	switch {
	case src == nil:
	case errors.Is(src, mongo.ErrNoDocuments):
		dst = fmt.Errorf("%w: %d", ErrNotFound, id)
	case mongo.IsDuplicateKeyError(src):
		dst = fmt.Errorf("%w: %d", ErrConflict, id)
	default:
		dst = fmt.Errorf("%w: %s", ErrInternal, src)
	}
	return
}

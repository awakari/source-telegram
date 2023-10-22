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
	Id int64 `bson:"id"`
}

const attrId = "id"

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
}
var sortGetBatch = bson.D{
	{
		Key:   attrId,
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

func (sm storageMongo) Exists(ctx context.Context, id int64) (exists bool, err error) {
	result := sm.coll.FindOne(ctx, bson.M{attrId: id}, optsGet)
	err = result.Err()
	switch {
	case err == nil:
		exists = true
	case errors.Is(err, mongo.ErrNoDocuments):
		err = nil
	default:
		err = decodeError(err, id)
	}
	return
}

func (sm storageMongo) GetPage(ctx context.Context, filter model.ChannelFilter, limit uint32, cursor int64) (page []int64, err error) {
	q := bson.M{
		attrId: bson.M{
			"$gt": cursor,
		},
	}
	if filter.IdDiv != 0 {
		q[attrId].(bson.M)["$mod"] = bson.A{
			filter.IdDiv,
			-int32(filter.IdRem),
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
				page = append(page, rec.Id)
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

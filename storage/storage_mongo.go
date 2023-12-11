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
	Id      int64  `bson:"id"`
	GroupId string `bson:"groupId"`
	UserId  string `bson:"userId"`
	Name    string `bson:"name"`
	Link    string `bson:"link"`
}

const attrId = "id"
const attrGroupId = "groupId"
const attrUserId = "userId"
const attrName = "name"
const attrLink = "link"

type storageMongo struct {
	conn *mongo.Client
	db   *mongo.Database
	coll *mongo.Collection
}

var optsSrvApi = options.ServerAPI(options.ServerAPIVersion1)
var optsGet = options.
	FindOne().
	SetShowRecordID(false).
	SetProjection(projGet)
var projGet = bson.D{
	{
		Key:   attrId,
		Value: 1,
	},
	{
		Key:   attrGroupId,
		Value: 1,
	},
	{
		Key:   attrUserId,
		Value: 1,
	},
	{
		Key:   attrName,
		Value: 1,
	},
	{
		Key:   attrLink,
		Value: 1,
	},
}
var sortGetBatchAsc = bson.D{
	{
		Key:   attrLink,
		Value: 1,
	},
}
var sortGetBatchDesc = bson.D{
	{
		Key:   attrLink,
		Value: -1,
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
				Key:   attrLink,
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

func (sm storageMongo) Create(ctx context.Context, ch model.Channel) (err error) {
	rec := recChan{
		Id:      ch.Id,
		GroupId: ch.GroupId,
		UserId:  ch.UserId,
		Name:    ch.Name,
		Link:    ch.Link,
	}
	_, err = sm.coll.InsertOne(ctx, rec)
	err = decodeError(err, ch.Link)
	return
}

func (sm storageMongo) Read(ctx context.Context, link string) (ch model.Channel, err error) {
	q := bson.M{
		attrLink: link,
	}
	var result *mongo.SingleResult
	result = sm.coll.FindOne(ctx, q, optsGet)
	err = result.Err()
	var rec recChan
	if err == nil {
		err = result.Decode(&rec)
	}
	if err == nil {
		ch.Id = rec.Id
		ch.GroupId = rec.GroupId
		ch.UserId = rec.UserId
		ch.Name = rec.Name
		ch.Link = rec.Link
	}
	err = decodeError(err, link)
	return
}

func (sm storageMongo) Delete(ctx context.Context, link string) (err error) {
	q := bson.M{
		attrLink: link,
	}
	var result *mongo.DeleteResult
	result, err = sm.coll.DeleteOne(ctx, q)
	switch err {
	case nil:
		if result.DeletedCount < 1 {
			err = fmt.Errorf("%w by link %s", ErrNotFound, link)
		}
	default:
		err = decodeError(err, link)
	}
	return
}

func (sm storageMongo) GetPage(ctx context.Context, filter model.ChannelFilter, limit uint32, cursor string, order model.Order) (page []model.Channel, err error) {
	q := bson.M{}
	if filter.IdDiv != 0 {
		q[attrId] = bson.M{
			"$mod": bson.A{
				filter.IdDiv,
				-int32(filter.IdRem),
			},
		}
	}
	if filter.UserId != "" {
		q[attrGroupId] = filter.GroupId
		q[attrUserId] = filter.UserId
	}
	optsList := options.
		Find().
		SetLimit(int64(limit)).
		SetShowRecordID(false).
		SetProjection(projGet)
	switch order {
	case model.OrderDesc:
		if cursor != "" {
			q[attrLink] = bson.M{
				"$lt": cursor,
			}
		}
		optsList = optsList.SetSort(sortGetBatchDesc)
	default:
		if cursor != "" {
			q[attrLink] = bson.M{
				"$gt": cursor,
			}
		}
		optsList = optsList.SetSort(sortGetBatchAsc)
	}
	var cur *mongo.Cursor
	cur, err = sm.coll.Find(ctx, q, optsList)
	if err == nil {
		var rec recChan
		for cur.Next(ctx) {
			err = errors.Join(err, cur.Decode(&rec))
			if err == nil {
				page = append(page, model.Channel{
					Id:      rec.Id,
					GroupId: rec.GroupId,
					UserId:  rec.UserId,
					Name:    rec.Name,
					Link:    rec.Link,
				})
			}
		}
	}
	err = decodeError(err, cursor)
	return
}

func decodeError(src error, link string) (dst error) {
	switch {
	case src == nil:
	case errors.Is(src, mongo.ErrNoDocuments):
		dst = fmt.Errorf("%w: %s", ErrNotFound, link)
	case mongo.IsDuplicateKeyError(src):
		dst = fmt.Errorf("%w: %s", ErrConflict, link)
	default:
		dst = fmt.Errorf("%w: %s", ErrInternal, src)
	}
	return
}

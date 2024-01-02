package service

import (
	"context"
	"fmt"
	"github.com/akurilov/go-tdlib/client"
	"github.com/awakari/source-telegram/model"
	"github.com/awakari/source-telegram/storage"
	"log/slog"
	"sync"
	"time"
)

type Service interface {
	Create(ctx context.Context, ch model.Channel) (err error)
	Read(ctx context.Context, link string) (ch model.Channel, err error)
	Delete(ctx context.Context, link string) (err error)
	GetPage(ctx context.Context, filter model.ChannelFilter, limit uint32, cursor string, order model.Order) (page []model.Channel, err error)

	RefreshJoinedLoop() (err error)
}

type service struct {
	clientTg        *client.Client
	stor            storage.Storage
	chansJoined     map[int64]model.Channel
	chansJoinedLock *sync.Mutex
	replicaRange    uint32
	replicaIndex    uint32
	log             *slog.Logger
}

const ListLimit = 1_000
const RefreshInterval = 5 * time.Minute

func NewService(
	clientTg *client.Client,
	stor storage.Storage,
	chansJoined map[int64]model.Channel,
	chansJoinedLock *sync.Mutex,
	replicaRange uint32,
	replicaIndex uint32,
	log *slog.Logger,
) Service {
	return service{
		clientTg:        clientTg,
		stor:            stor,
		chansJoined:     chansJoined,
		chansJoinedLock: chansJoinedLock,
		replicaRange:    replicaRange,
		replicaIndex:    replicaIndex,
		log:             log,
	}
}

func (svc service) Create(ctx context.Context, ch model.Channel) (err error) {
	var newChat *client.Chat
	newChat, err = svc.clientTg.SearchPublicChat(&client.SearchPublicChatRequest{
		Username: ch.Link,
	})
	if err == nil {
		if ch.Id == 0 {
			ch.Id = newChat.Id
		}
		if ch.Name != newChat.Title {
			ch.Name = newChat.Title
		}
		err = svc.stor.Create(ctx, ch)
	}
	return
}

func (svc service) Read(ctx context.Context, link string) (ch model.Channel, err error) {
	ch, err = svc.stor.Read(ctx, link)
	return
}

func (svc service) Delete(ctx context.Context, link string) (err error) {
	err = svc.stor.Delete(ctx, link)
	return
}

func (svc service) GetPage(ctx context.Context, filter model.ChannelFilter, limit uint32, cursor string, order model.Order) (page []model.Channel, err error) {
	page, err = svc.stor.GetPage(ctx, filter, limit, cursor, order)
	return
}

func (svc service) RefreshJoinedLoop() (err error) {
	ctx := context.TODO()
	for err == nil {
		err = svc.refreshJoined(ctx)
		if err == nil {
			time.Sleep(RefreshInterval)
		}
	}
	return
}

func (svc service) refreshJoined(ctx context.Context) (err error) {
	svc.log.Debug("Refresh joined channels started")
	defer svc.log.Debug("Refresh joined channels finished")
	// get all previously joined by the client chats
	var chatsJoined *client.Chats
	chatsJoined, err = svc.clientTg.GetChats(&client.GetChatsRequest{Limit: ListLimit})
	var chans []model.Channel
	if err == nil {
		svc.log.Debug(fmt.Sprintf("Refresh joined channels: got %d from the client", len(chatsJoined.ChatIds)))
		//
		chanFilter := model.ChannelFilter{
			IdDiv: svc.replicaRange,
			IdRem: svc.replicaIndex,
		}
		chans, err = svc.stor.GetPage(ctx, chanFilter, ListLimit, "", model.OrderAsc) // it's important to get all at once
	}
	if err == nil {
		svc.log.Debug(fmt.Sprintf("Refresh joined channels: got %d from the storage", len(chans)))
		for _, ch := range chans {
			var joined bool
			for _, chatJoinedId := range chatsJoined.ChatIds {
				if ch.Id == chatJoinedId {
					joined = true
					break
				}
			}
			if !joined {
				var newChat *client.Chat
				newChat, err = svc.clientTg.SearchPublicChat(&client.SearchPublicChatRequest{
					Username: ch.Link,
				})
				svc.log.Debug(fmt.Sprintf("SearchPublicChat(%s): %+v, %s", ch.Name, newChat, err))
				_, err = svc.clientTg.AddRecentlyFoundChat(&client.AddRecentlyFoundChatRequest{
					ChatId: ch.Id,
				})
				svc.log.Debug(fmt.Sprintf("AddRecentlyFoundChat(%d): %s", ch.Id, err))
				_, err = svc.clientTg.JoinChat(&client.JoinChatRequest{
					ChatId: ch.Id,
				})
				if err == nil {
					joined = true
				}
			}
			switch joined {
			case true:
				svc.log.Debug(fmt.Sprintf("Selected channel id: %d, title: %s, user: %s", ch.Id, ch.Name, ch.UserId))
				svc.addJoined(ch)
			default:
				svc.log.Warn(fmt.Sprintf("Failed to join channel by id: %d, cause: %s", ch.Id, err))
				err = nil
			}
		}
	}
	return
}

func (svc service) addJoined(ch model.Channel) {
	svc.chansJoinedLock.Lock()
	defer svc.chansJoinedLock.Unlock()
	svc.chansJoined[ch.Id] = ch
}

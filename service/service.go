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

type Service struct {
	ClientTg        *client.Client
	Storage         storage.Storage
	ChansJoined     map[int64]model.Channel
	ChansJoinedLock *sync.Mutex
	ReplicaRange    uint32
	ReplicaIndex    uint32
	Log             *slog.Logger
}

const ListLimit = 1_000
const RefreshInterval = 5 * time.Minute

func (svc Service) RefreshJoinedLoop() (err error) {
	ctx := context.TODO()
	for err == nil {
		err = svc.RefreshJoined(ctx)
		if err == nil {
			time.Sleep(RefreshInterval)
		}
	}
	return
}

func (svc Service) RefreshJoined(ctx context.Context) (err error) {
	svc.Log.Debug("Refresh joined channels started")
	defer svc.Log.Debug("Refresh joined channels finished")
	// get all previously joined by the client chats
	var chatsJoined *client.Chats
	chatsJoined, err = svc.ClientTg.GetChats(&client.GetChatsRequest{Limit: ListLimit})
	var chans []model.Channel
	if err == nil {
		svc.Log.Debug(fmt.Sprintf("Refresh joined channels: got %d from the client", len(chatsJoined.ChatIds)))
		//
		chanFilter := model.ChannelFilter{
			IdDiv: svc.ReplicaRange,
			IdRem: svc.ReplicaIndex,
		}
		chans, err = svc.Storage.GetPage(ctx, chanFilter, ListLimit, "") // it's important to get all at once
	}
	if err == nil {
		svc.Log.Debug(fmt.Sprintf("Refresh joined channels: got %d from the storage", len(chans)))
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
				newChat, err = svc.ClientTg.SearchPublicChat(&client.SearchPublicChatRequest{
					Username: ch.Link,
				})
				svc.Log.Debug(fmt.Sprintf("SearchPublicChat(%s): %+v, %s", ch.Name, newChat, err))
				_, err = svc.ClientTg.AddRecentlyFoundChat(&client.AddRecentlyFoundChatRequest{
					ChatId: ch.Id,
				})
				svc.Log.Debug(fmt.Sprintf("AddRecentlyFoundChat(%d): %s", ch.Id, err))
				_, err = svc.ClientTg.JoinChat(&client.JoinChatRequest{
					ChatId: ch.Id,
				})
				if err == nil {
					joined = true
				}
			}
			switch joined {
			case true:
				svc.Log.Debug(fmt.Sprintf("Selected channel id: %d, title: %s", ch.Id, ch.Name))
				svc.addJoined(ch)
			default:
				svc.Log.Warn(fmt.Sprintf("Failed to join channel by id: %d, cause: %s", ch.Id, err))
				err = nil
			}
		}
	}
	return
}

func (svc Service) addJoined(ch model.Channel) {
	svc.ChansJoinedLock.Lock()
	defer svc.ChansJoinedLock.Unlock()
	svc.ChansJoined[ch.Id] = ch
}

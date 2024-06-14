package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/akurilov/go-tdlib/client"
	"github.com/awakari/source-telegram/model"
	"github.com/awakari/source-telegram/storage"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Service interface {
	Create(ctx context.Context, ch model.Channel) (err error)
	Read(ctx context.Context, link string) (ch model.Channel, err error)
	Delete(ctx context.Context, link string) (err error)
	GetPage(ctx context.Context, filter model.ChannelFilter, limit uint32, cursor string, order model.Order) (page []model.Channel, err error)
	SearchAndAdd(ctx context.Context, groupId, subId, terms string, limit uint32) (n uint32, err error)

	RefreshJoinedLoop() (err error)
}

type service struct {
	clientTg        *client.Client
	stor            storage.Storage
	chansJoined     map[int64]*model.Channel
	chansJoinedLock *sync.Mutex
	log             *slog.Logger
	replicaIndex    int
}

const ListLimit = 1_000
const RefreshInterval = 15 * time.Minute
const minChanMemberCount = 2_345
const tagNoBot = "#nobot"

var ErrNoBot = fmt.Errorf("chat contains the %s tag in description", tagNoBot)

func NewService(
	clientTg *client.Client,
	stor storage.Storage,
	chansJoined map[int64]*model.Channel,
	chansJoinedLock *sync.Mutex,
	log *slog.Logger,
	replicaIndex int,
) Service {
	return service{
		clientTg:        clientTg,
		stor:            stor,
		chansJoined:     chansJoined,
		chansJoinedLock: chansJoinedLock,
		log:             log,
		replicaIndex:    replicaIndex,
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
		if svc.chatContainsNoBotTag(ch.Id) {
			err = fmt.Errorf("%w: %+v", ErrNoBot, ch)
		}
	}
	if err == nil {
		ch.Created = time.Now().UTC()
		ch.Last = ch.Created
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
		var chanFilter model.ChannelFilter
		if svc.replicaIndex > 0 {
			chanFilter.Label = strconv.Itoa(svc.replicaIndex)
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
				if svc.chatContainsNoBotTag(ch.Id) {
					svc.log.Debug(fmt.Sprintf("Channel contains the %s tag in the description, removing, id: %d, title: %s, user: %s", tagNoBot, ch.Id, ch.Name, ch.UserId))
					_ = svc.Delete(ctx, ch.Link)
				} else {
					svc.log.Debug(fmt.Sprintf("Selected channel id: %d, title: %s, user: %s", ch.Id, ch.Name, ch.UserId))
					svc.updateJoined(ctx, ch)
				}
			default:
				svc.log.Warn(fmt.Sprintf("Failed to join channel by id: %d, cause: %s", ch.Id, err))
				err = nil
			}
		}
	}
	return
}

func (svc service) updateJoined(ctx context.Context, ch model.Channel) {
	svc.chansJoinedLock.Lock()
	defer svc.chansJoinedLock.Unlock()
	chRuntime := svc.chansJoined[ch.Id]
	switch chRuntime {
	case nil:
		svc.chansJoined[ch.Id] = &ch
	default:
		if chRuntime.Last.After(ch.Last) {
			err := svc.stor.Update(ctx, ch.Link, chRuntime.Last)
			if err != nil {
				svc.log.Warn(fmt.Sprintf("Failed to update the channel %s last update time, cause: %s", ch.Link, err))
			}
		}
	}
	return
}

func (svc service) SearchAndAdd(ctx context.Context, groupId, subId, terms string, limit uint32) (n uint32, err error) {
	var chats *client.Chats
	chats, err = svc.clientTg.SearchPublicChats(&client.SearchPublicChatsRequest{
		Query: terms,
	})
	if err == nil && chats != nil {
		for i, chatId := range chats.ChatIds {
			if i >= int(limit) {
				break
			}
			var chat *client.Chat
			var chatErr error
			chat, chatErr = svc.clientTg.GetChat(&client.GetChatRequest{
				ChatId: chatId,
			})
			var sg *client.Supergroup
			if chatErr == nil && chat.Type.ChatTypeType() == client.TypeChatTypeSupergroup {
				sgChat := chat.Type.(*client.ChatTypeSupergroup)
				if svc.supergroupContainsNoBotTag(chatId, sgChat.SupergroupId) {
					err = errors.Join(err, fmt.Errorf("%w: \"%s\"", ErrNoBot, chat.Title))
					continue
				}
				if sgChat.IsChannel {
					sg, chatErr = svc.clientTg.GetSupergroup(&client.GetSupergroupRequest{
						SupergroupId: sgChat.SupergroupId,
					})
				} else {
					continue
				}
			}
			if chatErr == nil && sg != nil {
				var name string
				if sg.Usernames != nil && len(sg.Usernames.ActiveUsernames) > 0 {
					name = sg.Usernames.ActiveUsernames[0]
				}
				if name != "" && sg.MemberCount > minChanMemberCount {
					now := time.Now().UTC()
					chatErr = svc.stor.Create(ctx, model.Channel{
						Id:      chatId,
						GroupId: groupId,
						Name:    name,
						Link:    "@" + name,
						SubId:   subId,
						Terms:   terms,
						Last:    now,
						Created: now,
					})
				}
			}
			switch chatErr {
			case nil:
				n++
			default:
				err = errors.Join(err, chatErr)
			}
		}
	}
	return
}

func (svc service) chatContainsNoBotTag(chId int64) (contains bool) {
	var chat *client.Chat
	chat, err := svc.clientTg.GetChat(&client.GetChatRequest{
		ChatId: chId,
	})
	var sgChat *client.ChatTypeSupergroup
	if err == nil && chat.Type.ChatTypeType() == client.TypeChatTypeSupergroup {
		sgChat = chat.Type.(*client.ChatTypeSupergroup)
	}
	if err == nil && sgChat != nil {
		contains = svc.supergroupContainsNoBotTag(chId, sgChat.SupergroupId)
	}
	return
}

func (svc service) supergroupContainsNoBotTag(chId, sgId int64) (contains bool) {
	info, err := svc.clientTg.GetSupergroupFullInfo(&client.GetSupergroupFullInfoRequest{
		SupergroupId: sgId,
	})
	svc.log.Debug(fmt.Sprintf("GetSupergroupFullInfo(%d, %d): %+v, %s", chId, sgId, info, err))
	if err == nil && info != nil {
		for _, descrPart := range strings.Split(info.Description, " ") {
			if descrPart == tagNoBot {
				contains = true
				break
			}
		}
	}
	return
}

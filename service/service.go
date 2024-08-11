package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/akurilov/go-tdlib/client"
	"github.com/awakari/source-telegram/model"
	"github.com/awakari/source-telegram/storage"
	"github.com/cloudevents/sdk-go/binding/format/protobuf/v2/pb"
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
	HandleInterestChange(ctx context.Context, evt *pb.CloudEvent) (err error)

	RefreshJoinedLoop() (err error)
}

type service struct {
	clientTg        *client.Client
	stor            storage.Storage
	chansJoined     map[int64]*model.Channel
	chansJoinedLock *sync.Mutex
	log             *slog.Logger
	replicaIndex    int
	botUserId       int64
}

const ListLimit = 1_000
const RefreshInterval = 15 * time.Minute
const minChanMemberCount = 2_345
const TagNoBot = "#nobot"

const ceKeyGroupId = "awakarigroupid"
const ceKeyPublic = "public"
const ceKeyQueriesBasic = "queriesbasic"
const ceKeyDescription = "description"

var ErrNoBot = fmt.Errorf("chat/message contains the %s tag", TagNoBot)

func NewService(
	clientTg *client.Client,
	stor storage.Storage,
	chansJoined map[int64]*model.Channel,
	chansJoinedLock *sync.Mutex,
	log *slog.Logger,
	replicaIndex int,
	botUserId int64,
) Service {
	return service{
		clientTg:        clientTg,
		stor:            stor,
		chansJoined:     chansJoined,
		chansJoinedLock: chansJoinedLock,
		log:             log,
		replicaIndex:    replicaIndex,
		botUserId:       botUserId,
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
					svc.log.Debug(fmt.Sprintf("Channel contains the %s tag in the description, removing, id: %d, title: %s, user: %s", TagNoBot, ch.Id, ch.Name, ch.UserId))
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
			if descrPart == TagNoBot {
				contains = true
				break
			}
		}
	}
	return
}

func (svc service) HandleInterestChange(ctx context.Context, evt *pb.CloudEvent) (err error) {
	var groupId string
	if groupIdAttr, groupIdIdPresent := evt.Attributes[ceKeyGroupId]; groupIdIdPresent {
		groupId = groupIdAttr.GetCeString()
	}
	if groupId == "" {
		err = fmt.Errorf("interest %s event: empty group id, skipping", evt.GetTextData())
		return
	}
	publicAttr, publicAttrPresent := evt.Attributes[ceKeyPublic]
	if publicAttrPresent && publicAttr.GetCeBoolean() {
		err = svc.handlePublicInterestChange(ctx, evt)
	}
	return
}

func (svc service) handlePublicInterestChange(ctx context.Context, evt *pb.CloudEvent) (err error) {

	interestId := evt.GetTextData()
	var descr string
	if attrDescr, attrDescrPresent := evt.Attributes[ceKeyDescription]; attrDescrPresent {
		descr = attrDescr.GetCeString()
	}
	var cats []string
	if catsAttr, catsAttrPresent := evt.Attributes[ceKeyQueriesBasic]; catsAttrPresent {
		cats = strings.Split(catsAttr.GetCeString(), "\n")
	}
	var tags []string
	for _, cat := range cats {
		if strings.HasPrefix(cat, "#") {
			tags = append(tags, cat)
		} else {
			tags = append(tags, "#"+cat)
		}
	}
	name := strings.ToLower(interestId)
	name = strings.ReplaceAll(name, "-", "_")
	if len(name) > 28 {
		name = strings.ReplaceAll(name, "_", "")
	}
	if len(name) > 28 {
		name = name[:28]
	}
	name = "awk_" + name

	c, _ := svc.getPublicChan(name)
	switch c {
	case nil:
		c, err = svc.createChan(interestId, descr, tags)
		if err == nil {
			err = errors.Join(err, svc.setChanPublic(c, interestId, name))
			err = errors.Join(err, svc.setChanLogo(c, interestId))
		}
	}
	if err == nil {
		err = errors.Join(err, svc.setChanAdminBot(c, interestId))
		err = errors.Join(err, svc.subscribeChan(c, interestId))
	}

	return
}

func (svc service) getPublicChan(name string) (c *client.Chat, err error) {
	c, err = svc.clientTg.SearchPublicChat(&client.SearchPublicChatRequest{
		Username: name,
	})
	if err != nil || c.Type.ChatTypeType() != client.TypeChatTypeSupergroup || !c.Type.(*client.ChatTypeSupergroup).IsChannel {
		c = nil
	}
	return
}

func (svc service) createChan(interestId, descr string, tags []string) (c *client.Chat, err error) {
	c, err = svc.clientTg.CreateNewSupergroupChat(&client.CreateNewSupergroupChatRequest{
		Title:     descr,
		IsChannel: true,
		Description: fmt.Sprintf(
			"Awakari Interest: %s\nDetails: https://awakari.com/sub-details.html?id=%s\n%s",
			interestId, interestId, strings.Join(tags, " "),
		),
	})
	if err != nil {
		err = errors.Join(err, fmt.Errorf("failed to create a channel, interest=%s, err=%s", interestId, err))
	}
	return
}

func (svc service) setChanPublic(c *client.Chat, interestId, name string) (err error) {
	sgChat := c.Type.(*client.ChatTypeSupergroup)
	_, err = svc.clientTg.SetSupergroupUsername(&client.SetSupergroupUsernameRequest{
		SupergroupId: sgChat.SupergroupId,
		Username:     name,
	})
	if err != nil {
		err = errors.Join(err, fmt.Errorf("failed to set a channel name, interest=%s, err=%s", interestId, err))
	}
	return
}

func (svc service) setChanLogo(c *client.Chat, interestId string) (err error) {
	_, err = svc.clientTg.SetChatPhoto(&client.SetChatPhotoRequest{
		ChatId: c.Id,
		Photo: &client.InputChatPhotoStatic{
			Photo: &client.InputFileLocal{
				Path: "/logo.jpg",
			},
		},
	})
	if err != nil {
		err = errors.Join(err, fmt.Errorf("failed to set the channel photo, interest=%s, err=%s", interestId, err))
	}
	return
}

func (svc service) setChanAdminBot(c *client.Chat, interestId string) (err error) {
	_, err = svc.clientTg.SetChatMemberStatus(&client.SetChatMemberStatusRequest{
		ChatId: c.Id,
		MemberId: &client.MessageSenderUser{
			UserId: svc.botUserId,
		},
		Status: &client.ChatMemberStatusAdministrator{
			Rights: &client.ChatAdministratorRights{
				CanPostMessages:   true,
				CanEditMessages:   true,
				CanDeleteMessages: true,
				CanInviteUsers:    true,
				CanPinMessages:    true,
				CanPromoteMembers: true,
			},
		},
	})
	if err != nil {
		err = errors.Join(err, fmt.Errorf("failed to add the bot to the channel, interest=%s, err=%s", interestId, err))
	}
	return
}

func (svc service) subscribeChan(c *client.Chat, interestId string) (err error) {
	_, err = svc.clientTg.SendMessage(&client.SendMessageRequest{
		ChatId: c.Id,
		InputMessageContent: &client.InputMessageText{
			Text: &client.FormattedText{
				Text: fmt.Sprintf("/start %s", interestId),
				Entities: []*client.TextEntity{
					{
						Type: &client.TextEntityTypeBotCommand{},
					},
				},
			},
		},
	})
	if err != nil {
		err = errors.Join(err, fmt.Errorf("failed to send the channel message, interest=%s, chat=%d, err=%s", interestId, c.Id, err))
	}
	return
}

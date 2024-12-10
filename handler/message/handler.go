package message

import (
	"context"
	"errors"
	"fmt"
	"github.com/akurilov/go-tdlib/client"
	"github.com/awakari/source-telegram/api/http/pub"
	"github.com/awakari/source-telegram/handler"
	"github.com/awakari/source-telegram/model"
	"github.com/awakari/source-telegram/service"
	"github.com/cenkalti/backoff/v4"
	"github.com/cloudevents/sdk-go/binding/format/protobuf/v2/pb"
	"github.com/segmentio/ksuid"
	"google.golang.org/protobuf/types/known/timestamppb"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
)

type msgHandler struct {
	svcPub          pub.Service
	clientTg        *client.Client
	chansJoined     map[int64]*model.Channel
	chansJoinedLock *sync.Mutex
	log             *slog.Logger
	indexShard      int
}

type FileType int32

const (
	FileTypeUndefined FileType = iota
	FileTypeAudio
	FileTypeDocument
	FileTypeImage
	FileTypeVideo
)

const fmtAttrValType = "com_awakari_source_telegram_v1_%d"
const attrValSpecVersion = "1.0"
const attrKeyLatitude = "latitude"
const attrKeyLongitude = "longitude"
const attrKeyMsgId = "tgmessageid"
const attrKeyTime = "time"

// file attrs
const attrKeyFileId = "tgfileid"
const attrKeyFileUniqueId = "tgfileuniqueid"
const attrKeyFileMediaDuration = "tgfilemediaduration"
const attrKeyFileImgHeight = "tgfileimgheight"
const attrKeyFileImgWidth = "tgfileimgwidth"
const attrKeyFileType = "tgfiletype"

func NewHandler(
	svcPub pub.Service,
	clientTg *client.Client,
	chansJoined map[int64]*model.Channel,
	chansJoinedLock *sync.Mutex,
	log *slog.Logger,
	indexShard int,
) handler.Handler[*client.Message] {
	return msgHandler{
		svcPub:          svcPub,
		clientTg:        clientTg,
		chansJoined:     chansJoined,
		chansJoinedLock: chansJoinedLock,
		log:             log,
		indexShard:      indexShard,
	}
}

func (h msgHandler) Handle(ctx context.Context, msg *client.Message) (err error) {
	chanId := msg.ChatId
	evt := h.convertToEvent(chanId, msg)
	if evt != nil {
		err = h.updateChannelAndPublish(ctx, chanId, evt)
	}
	return
}

func (h msgHandler) convertToEvent(chanId int64, msg *client.Message) (evt *pb.CloudEvent) {
	if msg != nil {
		content := msg.Content
		if content != nil {
			var src string
			ch := h.chansJoined[chanId]
			if ch != nil {
				src = ch.Link
			}
			evt = &pb.CloudEvent{
				Id:          ksuid.New().String(),
				Source:      src,
				SpecVersion: attrValSpecVersion,
				Type:        fmt.Sprintf(fmtAttrValType, h.indexShard),
				Attributes: map[string]*pb.CloudEventAttributeValue{
					attrKeyMsgId: {
						Attr: &pb.CloudEventAttributeValue_CeString{
							CeString: strconv.FormatInt(msg.Id, 10),
						},
					},
					attrKeyTime: {
						Attr: &pb.CloudEventAttributeValue_CeTimestamp{
							CeTimestamp: timestamppb.New(time.Unix(int64(msg.Date), 0)),
						},
					},
				},
			}
			var err error
			switch content.MessageContentType() {
			case client.TypeMessageAudio:
				a := content.(*client.MessageAudio)
				convertAudio(a.Audio, evt)
				err = convertText(a.Caption, evt)
			case client.TypeMessageDocument:
				doc := content.(*client.MessageDocument)
				convertDocument(doc.Document, evt)
				err = convertText(doc.Caption, evt)
			case client.TypeMessageLocation:
				loc := content.(*client.MessageLocation)
				convertLocation(loc.Location, evt)
			case client.TypeMessagePhoto:
				img := content.(*client.MessagePhoto)
				convertImage(img.Photo.Sizes[0], evt)
				err = convertText(img.Caption, evt)
			case client.TypeMessageText:
				txt := content.(*client.MessageText)
				err = convertText(txt.Text, evt)
			case client.TypeMessageVideo:
				v := content.(*client.MessageVideo)
				convertVideo(v.Video, evt)
				err = convertText(v.Caption, evt)
			default:
				h.log.Info(fmt.Sprintf("unsupported message content type: %s\n", content.MessageContentType()))
			}
			switch err {
			case nil:
				h.log.Debug(fmt.Sprintf("New message %d from chat %d: converted to event id: %s, source: %s\n", msg.Id, msg.ChatId, evt.Id, evt.Source))
			default:
				h.log.Warn(fmt.Sprintf("Drop message %d from chat %d, cause: %s", msg.Id, msg.ChatId, err))
				evt = nil
			}
		}
	}
	return
}

func convertAudio(a *client.Audio, evt *pb.CloudEvent) {
	convertFile(a.Audio, evt)
	evt.Attributes[attrKeyFileType] = &pb.CloudEventAttributeValue{
		Attr: &pb.CloudEventAttributeValue_CeInteger{
			CeInteger: int32(FileTypeAudio),
		},
	}
	evt.Attributes[attrKeyFileMediaDuration] = &pb.CloudEventAttributeValue{
		Attr: &pb.CloudEventAttributeValue_CeInteger{
			CeInteger: a.Duration,
		},
	}
}

func convertDocument(doc *client.Document, evt *pb.CloudEvent) {
	convertFile(doc.Document, evt)
	evt.Attributes[attrKeyFileType] = &pb.CloudEventAttributeValue{
		Attr: &pb.CloudEventAttributeValue_CeInteger{
			CeInteger: int32(FileTypeDocument),
		},
	}
}

func convertLocation(loc *client.Location, evt *pb.CloudEvent) {
	evt.Attributes[attrKeyLatitude] = &pb.CloudEventAttributeValue{
		Attr: &pb.CloudEventAttributeValue_CeString{
			CeString: fmt.Sprintf("%f", loc.Latitude),
		},
	}
	evt.Attributes[attrKeyLongitude] = &pb.CloudEventAttributeValue{
		Attr: &pb.CloudEventAttributeValue_CeString{
			CeString: fmt.Sprintf("%f", loc.Longitude),
		},
	}
}

func convertImage(img *client.PhotoSize, evt *pb.CloudEvent) {
	convertFile(img.Photo, evt)
	evt.Attributes[attrKeyFileType] = &pb.CloudEventAttributeValue{
		Attr: &pb.CloudEventAttributeValue_CeInteger{
			CeInteger: int32(FileTypeImage),
		},
	}
	evt.Attributes[attrKeyFileImgHeight] = &pb.CloudEventAttributeValue{
		Attr: &pb.CloudEventAttributeValue_CeInteger{
			CeInteger: img.Height,
		},
	}
	evt.Attributes[attrKeyFileImgWidth] = &pb.CloudEventAttributeValue{
		Attr: &pb.CloudEventAttributeValue_CeInteger{
			CeInteger: img.Width,
		},
	}
}

func convertText(txt *client.FormattedText, evt *pb.CloudEvent) (err error) {
	for _, w := range strings.Split(txt.Text, " ") {
		if w == service.TagNoBot {
			err = service.ErrNoBot
			return
		}
	}
	evt.Data = &pb.CloudEvent_TextData{
		TextData: txt.Text,
	}
	return
}

func convertVideo(v *client.Video, evt *pb.CloudEvent) {
	convertFile(v.Video, evt)
	evt.Attributes[attrKeyFileType] = &pb.CloudEventAttributeValue{
		Attr: &pb.CloudEventAttributeValue_CeInteger{
			CeInteger: int32(FileTypeVideo),
		},
	}
	evt.Attributes[attrKeyFileMediaDuration] = &pb.CloudEventAttributeValue{
		Attr: &pb.CloudEventAttributeValue_CeInteger{
			CeInteger: v.Duration,
		},
	}
	evt.Attributes[attrKeyFileImgHeight] = &pb.CloudEventAttributeValue{
		Attr: &pb.CloudEventAttributeValue_CeInteger{
			CeInteger: v.Height,
		},
	}
	evt.Attributes[attrKeyFileImgWidth] = &pb.CloudEventAttributeValue{
		Attr: &pb.CloudEventAttributeValue_CeInteger{
			CeInteger: v.Width,
		},
	}
}

func convertFile(f *client.File, evt *pb.CloudEvent) {
	if f.Remote != nil {
		evt.Attributes[attrKeyFileId] = &pb.CloudEventAttributeValue{
			Attr: &pb.CloudEventAttributeValue_CeString{
				CeString: f.Remote.Id,
			},
		}
		evt.Attributes[attrKeyFileUniqueId] = &pb.CloudEventAttributeValue{
			Attr: &pb.CloudEventAttributeValue_CeString{
				CeString: f.Remote.UniqueId,
			},
		}
	}
}

func (h msgHandler) updateChannelAndPublish(ctx context.Context, chanId int64, evt *pb.CloudEvent) (err error) {
	h.chansJoinedLock.Lock()
	defer h.chansJoinedLock.Unlock()
	ch := h.chansJoined[chanId]
	switch ch {
	case nil:
		h.log.Debug(fmt.Sprintf("No joined channel found for id = %d", chanId))
	default:
		attrTs, attrTsOk := evt.Attributes[attrKeyTime]
		if attrTsOk && attrTs != nil {
			ts := attrTs.GetCeTimestamp()
			if ts != nil {
				ch.Last = ts.AsTime().UTC()
			}
		}
		groupId := ch.GroupId
		userId := ch.UserId
		if userId == "" {
			h.log.Debug(fmt.Sprintf("Channel %s has no assigned user id, using the channel id instead", ch.Link))
			userId = ch.Link
		}
		err = h.publish(ctx, evt, groupId, userId)
		switch {
		case err == nil:
		default:
			h.log.Error(fmt.Sprintf("Failed to publish event %s from channel %d, cause: %s", evt.Id, chanId, err))
		}
	}
	return
}

func (h msgHandler) publish(ctx context.Context, evt *pb.CloudEvent, groupId, userId string) (err error) {
	if evt.Data != nil {
		err = h.svcPub.Publish(ctx, evt, groupId, userId)
		if errors.Is(err, pub.ErrNoAck) {
			// retry with a backoff
			b := backoff.NewExponentialBackOff()
			b.InitialInterval = 100 * time.Millisecond
			b.MaxElapsedTime = 10 * time.Second
			err = backoff.RetryNotify(
				func() error {
					return h.svcPub.Publish(ctx, evt, groupId, userId)
				},
				b,
				func(err error, d time.Duration) {
					h.log.Warn(fmt.Sprintf("failed to write event %s, cause: %s, retrying in %s...", evt.Id, err, d))
				},
			)
		}
	}
	return
}

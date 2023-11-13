package message

import (
	"context"
	"errors"
	"fmt"
	"github.com/akurilov/go-tdlib/client"
	"github.com/awakari/client-sdk-go/api"
	"github.com/awakari/client-sdk-go/api/grpc/limits"
	modelAwk "github.com/awakari/client-sdk-go/model"
	"github.com/awakari/source-telegram/handler"
	"github.com/awakari/source-telegram/model"
	"github.com/cenkalti/backoff/v4"
	"github.com/cloudevents/sdk-go/binding/format/protobuf/v2/pb"
	"github.com/google/uuid"
	"google.golang.org/grpc/metadata"
	"log/slog"
	"strconv"
	"sync"
	"time"
)

type msgHandler struct {
	clientAwk       api.Client
	clientTg        *client.Client
	chansJoined     map[int64]model.Channel
	chansJoinedLock *sync.Mutex
	writers         map[int64]modelAwk.Writer[*pb.CloudEvent]
	log             *slog.Logger
}

type FileType int32

const (
	FileTypeUndefined FileType = iota
	FileTypeAudio
	FileTypeDocument
	FileTypeImage
	FileTypeVideo
)

const attrValType = "com.github.awakari.source-telegram"
const attrValSpecVersion = "1.0"
const attrKeyMsgId = "tgmessageid"

// file attrs
const attrKeyFileId = "tgfileid"
const attrKeyFileUniqueId = "tgfileuniqueid"
const attrKeyFileMediaDuration = "tgfilemediaduration"
const attrKeyFileImgHeight = "tgfileimgheight"
const attrKeyFileImgWidth = "tgfileimgwidth"
const attrKeyFileType = "tgfiletype"

var errNoAck = errors.New("event was not accepted")

func NewHandler(
	clientAwk api.Client,
	clientTg *client.Client,
	chansJoined map[int64]model.Channel,
	chansJoinedLock *sync.Mutex,
	log *slog.Logger,
) handler.Handler[*client.Message] {
	return msgHandler{
		clientAwk:       clientAwk,
		clientTg:        clientTg,
		chansJoined:     chansJoined,
		chansJoinedLock: chansJoinedLock,
		writers:         map[int64]modelAwk.Writer[*pb.CloudEvent]{},
		log:             log,
	}
}

func (h msgHandler) Handle(msg *client.Message) (err error) {
	chanId := msg.ChatId
	var w modelAwk.Writer[*pb.CloudEvent]
	w, ok := h.writers[chanId]
	if !ok {
		h.log.Debug(fmt.Sprintf("Writer not found for channel id = %d", chanId))
		var ch model.Channel
		//
		h.chansJoinedLock.Lock()
		ch, ok = h.chansJoined[chanId]
		h.chansJoinedLock.Unlock()
		//
		switch ok {
		case true:
			ctxGroupId := metadata.AppendToOutgoingContext(context.TODO(), "x-awakari-group-id", ch.GroupId)
			userId := ch.UserId
			if userId == "" {
				userId = ch.Link
			}
			w, err = h.clientAwk.OpenMessagesWriter(ctxGroupId, userId)
			if err == nil {
				h.log.Debug(fmt.Sprintf("New message writer is open for chanId=%d, groupId=%s, userId=%s", chanId, ch.GroupId, userId))
				h.writers[chanId] = w
			}
		default:
			h.log.Debug(fmt.Sprintf("No channel joined found for id = %d", chanId))
		}
	}
	if w != nil {
		err = h.handleMessage(w, msg)
		if err != nil {
			h.log.Warn(fmt.Sprintf("Message handle failure, closing the writer: %s", err))
			_ = w.Close()
			delete(h.writers, chanId)
		}
	}
	return
}

func (h msgHandler) Close() (err error) {
	for _, w := range h.writers {
		_ = w.Close()
	}
	clear(h.writers)
	return
}

func (h msgHandler) handleMessage(w modelAwk.Writer[*pb.CloudEvent], msg *client.Message) (err error) {
	//
	evt := &pb.CloudEvent{
		Id:          uuid.NewString(),
		SpecVersion: attrValSpecVersion,
		Type:        attrValType,
		Attributes: map[string]*pb.CloudEventAttributeValue{
			attrKeyMsgId: {
				Attr: &pb.CloudEventAttributeValue_CeString{
					CeString: strconv.FormatInt(msg.Id, 10),
				},
			},
		},
	}
	//
	h.convertSource(msg, evt)
	//
	content := msg.Content
	switch content.MessageContentType() {
	case client.TypeMessageAudio:
		a := content.(*client.MessageAudio)
		convertAudio(a.Audio, evt)
		convertText(a.Caption, evt)
	case client.TypeMessageDocument:
		doc := content.(*client.MessageDocument)
		convertDocument(doc.Document, evt)
		convertText(doc.Caption, evt)
	case client.TypeMessagePhoto:
		img := content.(*client.MessagePhoto)
		convertImage(img.Photo.Sizes[0], evt)
		convertText(img.Caption, evt)
	case client.TypeMessageText:
		txt := content.(*client.MessageText)
		convertText(txt.Text, evt)
	case client.TypeMessageVideo:
		v := content.(*client.MessageVideo)
		convertVideo(v.Video, evt)
		convertText(v.Caption, evt)
	default:
		h.log.Info(fmt.Sprintf("unsupported message content type: %s\n", content.MessageContentType()))
	}
	//
	if evt.Data != nil {
		h.log.Debug(fmt.Sprintf("New message %d from chat %d: converted to event id: %s, source: %s\n", msg.Id, msg.ChatId, evt.Id, evt.Source))
		evts := []*pb.CloudEvent{
			evt,
		}
		err = h.tryWriteEventOnce(w, evts)
		if err != nil {
			// retry with a backoff
			b := backoff.NewExponentialBackOff()
			b.InitialInterval = 100 * time.Millisecond
			b.MaxElapsedTime = 100 * time.Second
			err = backoff.RetryNotify(
				func() error {
					return h.tryWriteEventOnce(w, evts)
				},
				b,
				func(err error, d time.Duration) {
					h.log.Warn(fmt.Sprintf("Failed to write event %s, cause: %s, retrying in %s...", evt.Id, err, d))
				},
			)
		}
	}
	return
}

func (h msgHandler) tryWriteEventOnce(w modelAwk.Writer[*pb.CloudEvent], evts []*pb.CloudEvent) (err error) {
	var ackCount uint32
	ackCount, err = w.WriteBatch(evts)
	switch {
	case err == nil:
		if ackCount < 1 {
			err = errNoAck // it's an error to retry
		}
	case errors.Is(err, limits.ErrReached):
		h.log.Warn(fmt.Sprintf("Dropping the event %s, daily limit reached: %s", evts[0].Id, err))
		err = nil
	}
	return
}

func (h msgHandler) convertSource(msg *client.Message, evt *pb.CloudEvent) {
	var link *client.MessageLink
	var err error
	link, err = h.clientTg.GetMessageLink(&client.GetMessageLinkRequest{
		ChatId:    msg.ChatId,
		MessageId: msg.Id,
	})
	switch err {
	case nil:
		evt.Source = link.Link
	default:
		h.log.Warn(fmt.Sprintf("Failed to get the message %d from chat %d link: %s", msg.Id, msg.ChatId, err))
		evt.Source = strconv.FormatInt(msg.Id, 10)
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

func convertText(txt *client.FormattedText, evt *pb.CloudEvent) {
	evt.Data = &pb.CloudEvent_TextData{
		TextData: txt.Text,
	}
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

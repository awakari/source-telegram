package message

import (
	"context"
	"errors"
	"fmt"
	"github.com/akurilov/go-tdlib/client"
	"github.com/awakari/client-sdk-go/api"
	"github.com/awakari/client-sdk-go/api/grpc/limits"
	"github.com/awakari/client-sdk-go/api/grpc/permits"
	"github.com/awakari/client-sdk-go/api/grpc/resolver"
	modelAwk "github.com/awakari/client-sdk-go/model"
	"github.com/awakari/source-telegram/handler"
	"github.com/awakari/source-telegram/model"
	"github.com/awakari/source-telegram/service"
	"github.com/cenkalti/backoff/v4"
	"github.com/cloudevents/sdk-go/binding/format/protobuf/v2/pb"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/segmentio/ksuid"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
)

type msgHandler struct {
	clientAwk       api.Client
	clientTg        *client.Client
	chansJoined     map[int64]*model.Channel
	chansJoinedLock *sync.Mutex
	writers         *expirable.LRU[int64, modelAwk.Writer[*pb.CloudEvent]]
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

const fmtAttrValType = "com.awakari.source-telegram-%d.v1"
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

const writerCacheTtl = 15 * time.Minute
const writerCacheSize = 1_000

var errNoAck = errors.New("event was not accepted")

func NewHandler(
	clientAwk api.Client,
	clientTg *client.Client,
	chansJoined map[int64]*model.Channel,
	chansJoinedLock *sync.Mutex,
	log *slog.Logger,
	indexShard int,
) handler.Handler[*client.Message] {
	return msgHandler{
		clientAwk:       clientAwk,
		clientTg:        clientTg,
		chansJoined:     chansJoined,
		chansJoinedLock: chansJoinedLock,
		writers:         expirable.NewLRU[int64, modelAwk.Writer[*pb.CloudEvent]](writerCacheSize, evictWriter, writerCacheTtl),
		log:             log,
		indexShard:      indexShard,
	}
}

func evictWriter(_ int64, w modelAwk.Writer[*pb.CloudEvent]) {
	_ = w.Close()
}

func (h msgHandler) Handle(msg *client.Message) (err error) {
	chanId := msg.ChatId
	evt := h.convertToEvent(chanId, msg)
	if evt != nil {
		err = h.getWriterAndPublish(chanId, evt)
		if err != nil {
			// retry with a backoff
			b := backoff.NewExponentialBackOff()
			b.InitialInterval = 100 * time.Millisecond
			b.MaxElapsedTime = 10 * time.Second
			err = backoff.RetryNotify(
				func() error {
					return h.getWriterAndPublish(chanId, evt)
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

func (h msgHandler) Close() (err error) {
	for _, k := range h.writers.Keys() {
		w, found := h.writers.Get(k)
		if found {
			_ = w.Close()
		}
	}
	h.writers.Purge()
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

func (h msgHandler) getWriterAndPublish(chanId int64, evt *pb.CloudEvent) (err error) {
	var w modelAwk.Writer[*pb.CloudEvent]
	w, err = h.getWriterAndUpdateChannel(chanId, evt.Attributes[attrKeyTime])
	if w != nil && err == nil {
		err = h.publish(w, evt)
		switch {
		case err == nil:
		case errors.Is(err, limits.ErrReached):
			fallthrough
		case errors.Is(err, limits.ErrUnavailable):
			fallthrough
		case errors.Is(err, limits.ErrInternal):
			fallthrough
		case errors.Is(err, permits.ErrUnavailable):
			fallthrough
		case errors.Is(err, permits.ErrInternal):
			fallthrough
		case errors.Is(err, resolver.ErrUnavailable):
			fallthrough
		case errors.Is(err, io.EOF):
			h.log.Warn(fmt.Sprintf("Closing the failing writer stream for channeld %d before retrying, cause: %s", chanId, err))
			h.chansJoinedLock.Lock()
			defer h.chansJoinedLock.Unlock()
			_ = w.Close()
			h.writers.Remove(chanId)
		default:
			h.log.Error(fmt.Sprintf("Failed to publish event %s from channel %d, cause: %s", evt.Id, chanId, err))
		}
	}
	return
}

func (h msgHandler) getWriterAndUpdateChannel(
	chanId int64,
	attrTs *pb.CloudEventAttributeValue,
) (
	w modelAwk.Writer[*pb.CloudEvent],
	err error,
) {
	h.chansJoinedLock.Lock()
	defer h.chansJoinedLock.Unlock()
	var ok bool
	w, ok = h.writers.Get(chanId)
	if !ok {
		h.log.Debug(fmt.Sprintf("Writer not found for channel id = %d", chanId))
		ch := h.chansJoined[chanId]
		switch ch {
		case nil:
			h.log.Debug(fmt.Sprintf("No joined channel found for id = %d", chanId))
		default:
			if attrTs != nil {
				ts := attrTs.GetCeTimestamp()
				if ts != nil {
					ch.Last = ts.AsTime().UTC()
				}
			}
			ctxGroupId := metadata.AppendToOutgoingContext(context.TODO(), "x-awakari-group-id", ch.GroupId)
			userId := ch.UserId
			if userId == "" {
				h.log.Debug(fmt.Sprintf("Channel %s has no assigned user id, using the channel id instead", ch.Link))
				userId = ch.Link
			}
			w, err = h.clientAwk.OpenMessagesWriter(ctxGroupId, userId)
			if err == nil {
				h.log.Debug(fmt.Sprintf("New message writer is open for chanId=%d, groupId=%s, userId=%s", chanId, ch.GroupId, userId))
				h.writers.Add(chanId, w)
			}
		}
	}
	return
}

func (h msgHandler) publish(w modelAwk.Writer[*pb.CloudEvent], evt *pb.CloudEvent) (err error) {
	if evt.Data != nil {
		evts := []*pb.CloudEvent{
			evt,
		}
		err = h.tryWriteEventOnce(w, evts)
		if err != nil {
			// retry with a backoff
			b := backoff.NewExponentialBackOff()
			b.InitialInterval = 100 * time.Millisecond
			switch {
			case errors.Is(err, limits.ErrReached):
				// this error may be due to internal gRPC status "resource exhausted", try to reopen the writer
				fallthrough
			case errors.Is(err, limits.ErrUnavailable):
				fallthrough
			case errors.Is(err, permits.ErrUnavailable):
				fallthrough
			case errors.Is(err, resolver.ErrUnavailable):
				fallthrough
			case errors.Is(err, resolver.ErrInternal):
				// avoid retrying this before reopening the writer
			default:
				b.MaxElapsedTime = 10 * time.Second
				err = backoff.RetryNotify(
					func() error {
						return h.tryWriteEventOnce(w, evts)
					},
					b,
					func(err error, d time.Duration) {
						h.log.Warn(fmt.Sprintf("failed to write event %s, cause: %s, retrying in %s...", evt.Id, err, d))
					},
				)
			}
		}
	}
	return
}

func (h msgHandler) tryWriteEventOnce(w modelAwk.Writer[*pb.CloudEvent], evts []*pb.CloudEvent) (err error) {
	var ackCount uint32
	ackCount, err = w.WriteBatch(evts)
	if err == nil && ackCount < 1 {
		err = errNoAck // it's an error to retry
	}
	return
}

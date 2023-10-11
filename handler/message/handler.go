package message

import (
	"errors"
	"fmt"
	"github.com/awakari/client-sdk-go/model"
	"github.com/awakari/producer-telegram/handler"
	"github.com/cenkalti/backoff/v4"
	"github.com/cloudevents/sdk-go/binding/format/protobuf/v2/pb"
	"github.com/google/uuid"
	"github.com/zelenin/go-tdlib/client"
	"log/slog"
	"strconv"
)

type msgHandler struct {
	chatLinkById map[int64]string
	writerAwk    model.Writer[*pb.CloudEvent]
	b            *backoff.ExponentialBackOff
	log          *slog.Logger
}

type FileType int32

const (
	FileTypeUndefined FileType = iota
	FileTypeAudio
	FileTypeDocument
	FileTypeImage
	FileTypeVideo
)

const attrValType = "com.github.awakari.producer-telegram"
const attrValSpecVersion = "1.0"
const attrKeyMsgId = "telegrammessageid"
const fmtLinkChatMsg = "\"%s\""

// file attrs
const attrKeyFileId = "tgfileid"
const attrKeyFileUniqueId = "tgfileuniqueid"
const attrKeyFileMediaDuration = "tgfilemediaduration"
const attrKeyFileImgHeight = "tgfileimgheight"
const attrKeyFileImgWidth = "tgfileimgwidth"
const attrKeyFileType = "tgfiletype"

var errNoAck = errors.New("event was not accepted")

func NewHandler(chatLinkById map[int64]string, writerAwk model.Writer[*pb.CloudEvent], log *slog.Logger) handler.Handler[*client.Message] {
	return msgHandler{
		chatLinkById: chatLinkById,
		writerAwk:    writerAwk,
		b:            backoff.NewExponentialBackOff(),
		log:          log,
	}
}

func (h msgHandler) Handle(msg *client.Message) (err error) {
	chatLink, chatOk := h.chatLinkById[msg.ChatId]
	if chatOk {
		err = h.handleMessage(chatLink, msg)
	}
	return
}

func (h msgHandler) handleMessage(chatLink string, msg *client.Message) (err error) {
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
	convertSource(msg, chatLink, evt)
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
		h.log.Info(fmt.Sprintf("New message id %d from chat %d: converted to event id = %s\n", msg.Id, msg.ChatId, evt.Id))
		evts := []*pb.CloudEvent{
			evt,
		}
		err = backoff.Retry(
			func() (err error) {
				var ackCount uint32
				ackCount, err = h.writerAwk.WriteBatch(evts)
				if err == nil && ackCount < 1 {
					err = errNoAck
				}
				return
			},
			h.b,
		)
	}
	return
}

func convertSource(msg *client.Message, chatLink string, evt *pb.CloudEvent) {
	senderId := msg.SenderId
	switch senderId.MessageSenderType() {
	case client.TypeMessageSenderChat:
		evt.Source = chatLink
	}
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

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
	chatNameById map[int64]string
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

func NewHandler(chatNameById map[int64]string, writerAwk model.Writer[*pb.CloudEvent], log *slog.Logger) handler.Handler[*client.Message] {
	return msgHandler{
		chatNameById: chatNameById,
		writerAwk:    writerAwk,
		b:            backoff.NewExponentialBackOff(),
		log:          log,
	}
}

func (h msgHandler) Handle(msg *client.Message) (err error) {
	chatName, chatOk := h.chatNameById[msg.ChatId]
	if chatOk {
		err = h.handleMessage(chatName, msg)
	}
	return
}

func (h msgHandler) handleMessage(chatName string, msg *client.Message) (err error) {
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
	convertSource(msg, chatName, evt)
	//
	content := msg.Content
	switch content.MessageContentType() {
	case client.TypeMessageAudio:
		a := content.(*client.MessageAudio)
		h.log.Info(fmt.Sprintf(
			"chat: \"%s\", message: %d, audio, caption: %s, file id: %d, title: %s, duration: %d",
			chatName, msg.Id, a.Caption.Text, a.Audio.Audio.Id, a.Audio.Title, a.Audio.Duration,
		))
		convertAudio(a.Audio, evt)
		convertText(a.Caption, evt)
	case client.TypeMessageDocument:
		doc := content.(*client.MessageDocument)
		h.log.Info(fmt.Sprintf(
			"chat: \"%s\", message: %d, document, caption: %s, file id: %d",
			chatName, msg.Id, doc.Caption.Text, doc.Document.Document.Id,
		))
		convertDocument(doc.Document, evt)
		convertText(doc.Caption, evt)
	case client.TypeMessagePhoto:
		img := content.(*client.MessagePhoto)
		h.log.Info(fmt.Sprintf(
			"chat: \"%s\", message: %d, image, caption: %s, file id: %d, width: %d, height: %d",
			chatName, msg.Id, img.Caption.Text, img.Photo.Sizes[0].Photo.Id, img.Photo.Sizes[0].Width, img.Photo.Sizes[0].Height,
		))
		convertImage(img.Photo.Sizes[0], evt)
		convertText(img.Caption, evt)
	case client.TypeMessageText:
		txt := content.(*client.MessageText)
		h.log.Info(fmt.Sprintf("chat: \"%s\", message: %d, text: %s", chatName, msg.Id, txt.Text.Text))
		convertText(txt.Text, evt)
	case client.TypeMessageVideo:
		v := content.(*client.MessageVideo)
		h.log.Info(fmt.Sprintf(
			"chat: \"%s\", message: %d, v, caption: %s, file id: %d, duration: %d, width: %d, height: %d",
			chatName, msg.Id, v.Caption.Text, v.Video.Video.Id, v.Video.Duration, v.Video.Width, v.Video.Height,
		))
		convertVideo(v.Video, evt)
		convertText(v.Caption, evt)
	default:
		h.log.Info(fmt.Sprintf("unsupported message content type: %s\n", content.MessageContentType()))
	}
	//
	if evt.Data != nil {
		h.log.Info(fmt.Sprintf("New message id %d: converted to event id = %s\n", msg.Id, evt.Id))
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

func convertSource(msg *client.Message, chatName string, evt *pb.CloudEvent) {
	senderId := msg.SenderId
	switch senderId.MessageSenderType() {
	case client.TypeMessageSenderChat:
		evt.Source = fmt.Sprintf(fmtLinkChatMsg, chatName)
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

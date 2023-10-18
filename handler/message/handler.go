package message

import (
	"errors"
	"fmt"
	"github.com/Arman92/go-tdlib"
	"github.com/awakari/client-sdk-go/model"
	"github.com/awakari/source-telegram/handler"
	"github.com/cenkalti/backoff/v4"
	"github.com/cloudevents/sdk-go/binding/format/protobuf/v2/pb"
	"github.com/google/uuid"
	"log/slog"
	"strconv"
)

type msgHandler struct {
	clientTg  *tdlib.Client
	chats     map[int64]bool
	writerAwk model.Writer[*pb.CloudEvent]
	b         *backoff.ExponentialBackOff
	log       *slog.Logger
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

func NewHandler(clientTg *tdlib.Client, chats map[int64]bool, writerAwk model.Writer[*pb.CloudEvent], log *slog.Logger) handler.Handler[*tdlib.UpdateNewMessage] {
	return msgHandler{
		clientTg:  clientTg,
		chats:     chats,
		writerAwk: writerAwk,
		b:         backoff.NewExponentialBackOff(),
		log:       log,
	}
}

func (h msgHandler) Handle(msg *tdlib.UpdateNewMessage) (err error) {
	_, chatOk := h.chats[msg.Message.ChatID]
	if chatOk {
		err = h.handleMessage(msg.Message)
	}
	return
}

func (h msgHandler) handleMessage(msg *tdlib.Message) (err error) {
	//
	evt := &pb.CloudEvent{
		Id:          uuid.NewString(),
		SpecVersion: attrValSpecVersion,
		Type:        attrValType,
		Attributes: map[string]*pb.CloudEventAttributeValue{
			attrKeyMsgId: {
				Attr: &pb.CloudEventAttributeValue_CeString{
					CeString: strconv.FormatInt(msg.ID, 10),
				},
			},
		},
	}
	//
	err = h.convertSource(msg, evt)
	//
	content := msg.Content
	switch content.GetMessageContentEnum() {
	case tdlib.MessageAudioType:
		a := content.(*tdlib.MessageAudio)
		convertAudio(a.Audio, evt)
		convertText(a.Caption, evt)
	case tdlib.MessageDocumentType:
		doc := content.(*tdlib.MessageDocument)
		convertDocument(doc.Document, evt)
		convertText(doc.Caption, evt)
	case tdlib.MessagePhotoType:
		img := content.(*tdlib.MessagePhoto)
		convertImage(img.Photo.Sizes[0], evt)
		convertText(img.Caption, evt)
	case tdlib.MessageTextType:
		txt := content.(*tdlib.MessageText)
		convertText(txt.Text, evt)
	case tdlib.MessageVideoType:
		v := content.(*tdlib.MessageVideo)
		convertVideo(v.Video, evt)
		convertText(v.Caption, evt)
	default:
		h.log.Info(fmt.Sprintf("unsupported message content type: %s\n", content.GetMessageContentEnum()))
	}
	//
	if evt.Data != nil {
		h.log.Debug(fmt.Sprintf("New message %d from chat %d: converted to event id: %s, source: %s\n", msg.ID, msg.ChatID, evt.Id, evt.Source))
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

func (h msgHandler) convertSource(msg *tdlib.Message, evt *pb.CloudEvent) (err error) {
	var link *tdlib.MessageLink
	link, err = h.clientTg.GetMessageLink(msg.ChatID, msg.ID, false, false)
	switch err {
	case nil:
		evt.Source = link.Link
	default:
		evt.Source = strconv.FormatInt(msg.ID, 10)
	}
	return
}

func convertAudio(a *tdlib.Audio, evt *pb.CloudEvent) {
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

func convertDocument(doc *tdlib.Document, evt *pb.CloudEvent) {
	convertFile(doc.Document, evt)
	evt.Attributes[attrKeyFileType] = &pb.CloudEventAttributeValue{
		Attr: &pb.CloudEventAttributeValue_CeInteger{
			CeInteger: int32(FileTypeDocument),
		},
	}
}

func convertImage(img tdlib.PhotoSize, evt *pb.CloudEvent) {
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

func convertText(txt *tdlib.FormattedText, evt *pb.CloudEvent) {
	evt.Data = &pb.CloudEvent_TextData{
		TextData: txt.Text,
	}
}

func convertVideo(v *tdlib.Video, evt *pb.CloudEvent) {
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

func convertFile(f *tdlib.File, evt *pb.CloudEvent) {
	if f.Remote != nil {
		evt.Attributes[attrKeyFileId] = &pb.CloudEventAttributeValue{
			Attr: &pb.CloudEventAttributeValue_CeString{
				CeString: f.Remote.ID,
			},
		}
		evt.Attributes[attrKeyFileUniqueId] = &pb.CloudEventAttributeValue{
			Attr: &pb.CloudEventAttributeValue_CeString{
				CeString: f.Remote.UniqueID,
			},
		}
	}
}

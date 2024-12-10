package update

import (
	"context"
	"fmt"
	"github.com/akurilov/go-tdlib/client"
	"github.com/awakari/source-telegram/handler"
	"log/slog"
)

type ListenerHandler interface {
	handler.Handler[client.Type]
	Listen(ctx context.Context) (err error)
}

type updateHandler struct {
	listener   *client.Listener
	msgHandler handler.Handler[*client.Message]
	log        *slog.Logger
}

func NewHandler(listener *client.Listener, msgHandler handler.Handler[*client.Message], log *slog.Logger) ListenerHandler {
	return updateHandler{
		listener:   listener,
		msgHandler: msgHandler,
		log:        log,
	}
}

func (h updateHandler) Handle(ctx context.Context, u client.Type) (err error) {
	switch u.GetClass() {
	case client.ClassUpdate:
		switch u.GetType() {
		case client.TypeUpdateNewMessage:
			msg := u.(*client.UpdateNewMessage).Message
			if !msg.IsOutgoing {
				err = h.msgHandler.Handle(ctx, u.(*client.UpdateNewMessage).Message)
			}
		}
	}
	return
}

func (h updateHandler) Listen(ctx context.Context) (err error) {
	defer h.log.Info("Exit receiving updates")
	for u := range h.listener.Updates {
		err = h.Handle(ctx, u)
		if err != nil {
			h.log.Error(fmt.Sprintf("Failed to handle the update %+v, cause: %s", u, err))
		}
	}
	return
}

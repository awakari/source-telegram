package update

import (
	"fmt"
	"github.com/akurilov/go-tdlib/client"
	"github.com/awakari/source-telegram/handler"
	"github.com/cenkalti/backoff/v4"
	"log/slog"
	"time"
)

type ListenerHandler interface {
	handler.Handler[client.Type]
	Listen() (err error)
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

func (h updateHandler) Handle(u client.Type) (err error) {
	switch u.GetClass() {
	case client.ClassUpdate:
		switch u.GetType() {
		case client.TypeUpdateNewMessage:
			msg := u.(*client.UpdateNewMessage).Message
			if !msg.IsOutgoing {
				err = h.msgHandler.Handle(u.(*client.UpdateNewMessage).Message)
			}
		}
	}
	return
}

func (h updateHandler) Listen() (err error) {
	for u := range h.listener.Updates {
		err = h.Handle(u)
		if err != nil {
			// retry with a backoff
			b := backoff.NewExponentialBackOff()
			err = backoff.RetryNotify(
				func() error {
					return h.Handle(u)
				},
				b,
				func(err error, d time.Duration) {
					h.log.Warn(fmt.Sprintf("Failed to handle the update %+v, cause: %s, retrying in %s...", u, err, d))
				},
			)
		}
		if err != nil {
			h.log.Error(fmt.Sprintf("Failed to handle the update %+v, cause: %s", u, err))
		}
	}
	return
}

func (h updateHandler) Close() error {
	return nil
}

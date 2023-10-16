package update

import (
	"github.com/awakari/source-telegram/handler"
	"github.com/zelenin/go-tdlib/client"
)

type ListenerHandler interface {
	handler.Handler[client.Type]
	Listen() (err error)
}

type updateHandler struct {
	listener   *client.Listener
	msgHandler handler.Handler[*client.Message]
}

func NewHandler(listener *client.Listener, msgHandler handler.Handler[*client.Message]) ListenerHandler {
	return updateHandler{
		listener:   listener,
		msgHandler: msgHandler,
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
			break
		}
	}
	return
}

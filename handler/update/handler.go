package update

import (
	"github.com/awakari/producer-telegram/handler"
	"github.com/zelenin/go-tdlib/client"
)

type ListenerHandler interface {
	handler.Handler[client.Type]
	Listen() (err error)
}

type updateHandler struct {
	listener   *client.Listener
	msgHandler handler.Handler[*client.UpdateChatLastMessage]
}

func NewHandler(listener *client.Listener, msgHandler handler.Handler[*client.UpdateChatLastMessage]) ListenerHandler {
	return updateHandler{
		listener:   listener,
		msgHandler: msgHandler,
	}
}

func (h updateHandler) Handle(u client.Type) (err error) {
	switch u.GetClass() {
	case client.ClassUpdate:
		switch u.GetType() {
		case client.TypeUpdateChatLastMessage:
			err = h.msgHandler.Handle(u.(*client.UpdateChatLastMessage))
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

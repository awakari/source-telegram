package update

import (
	"fmt"
	"github.com/awakari/producer-telegram/handler"
	"github.com/zelenin/go-tdlib/client"
)

type ListenerHandler interface {
	handler.Handler[client.Type]
	Listen() (err error)
}

type updateHandler struct {
	listener   *client.Listener
	msgHandler handler.Handler[*client.UpdateNewMessage]
}

func NewHandler(listener *client.Listener, msgHandler handler.Handler[*client.UpdateNewMessage]) ListenerHandler {
	return updateHandler{
		listener:   listener,
		msgHandler: msgHandler,
	}
}

func (h updateHandler) Handle(u client.Type) (err error) {
	switch u.GetClass() {
	case client.ClassUpdate:
		fmt.Printf("new update: %+v\n", u)
		switch u.GetType() {
		case client.TypeUpdateNewMessage:
			err = h.msgHandler.Handle(u.(*client.UpdateNewMessage))
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

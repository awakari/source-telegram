package handler

import (
	"github.com/zelenin/go-tdlib/client"
)

type Handler[U client.Type] interface {
	Handle(u U) (err error)
}

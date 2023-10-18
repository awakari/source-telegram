package handler

import (
	"github.com/akurilov/go-tdlib/client"
)

type Handler[U client.Type] interface {
	Handle(u U) (err error)
}

package handler

import (
	"github.com/akurilov/go-tdlib/client"
	"io"
)

type Handler[U client.Type] interface {
	io.Closer
	Handle(u U) (err error)
}

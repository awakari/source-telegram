package handler

import (
	"github.com/Arman92/go-tdlib"
)

type Handler[U tdlib.Update] interface {
	Handle(u U) (err error)
}

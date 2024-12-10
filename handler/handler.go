package handler

import (
	"context"
	"github.com/akurilov/go-tdlib/client"
)

type Handler[U client.Type] interface {
	Handle(ctx context.Context, u U) (err error)
}

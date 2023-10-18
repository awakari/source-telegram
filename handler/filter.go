package handler

import (
	"fmt"
	"github.com/Arman92/go-tdlib"
	"github.com/cenkalti/backoff/v4"
	"time"
)

type ChatFilterHandler struct {
	Client     *tdlib.Client
	Chats      map[int64]bool
	MsgHandler Handler[*tdlib.UpdateNewMessage]
	QueueSize  int
	BackOff    *backoff.ExponentialBackOff
}

func (cfh ChatFilterHandler) Run() (err error) {
	// Here we can add a receiver to retreive any message type we want
	// We like to get UpdateNewMessage events and with a specific FilterFunc
	rcvr := cfh.Client.AddEventReceiver(&tdlib.UpdateNewMessage{}, cfh.filterByChat, cfh.QueueSize)
	run := func() error {
		return cfh.run(rcvr)
	}
	err = backoff.RetryNotify(run, cfh.BackOff, notify)
	return
}

func (cfh ChatFilterHandler) run(rcvr tdlib.EventReceiver) (err error) {
	for newMsg := range rcvr.Chan {
		fmt.Println(newMsg)
		updateMsg := (newMsg).(*tdlib.UpdateNewMessage)
		err = cfh.MsgHandler.Handle(updateMsg)
		if err != nil {
			break
		}
	}
	return
}

func (cfh ChatFilterHandler) filterByChat(msg *tdlib.TdMessage) (ok bool) {
	updateMsg := (*msg).(*tdlib.UpdateNewMessage)
	_, ok = cfh.Chats[updateMsg.Message.ChatID]
	return
}

func notify(err error, d time.Duration) {
	fmt.Printf("Failed to handle an update, cause: %s, retrying in: %s...\n", err, d)
}

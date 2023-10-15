package main

import (
	"context"
	"fmt"
	"github.com/awakari/producer-telegram/config"
	"github.com/awakari/producer-telegram/handler/message"
	"github.com/awakari/producer-telegram/handler/update"
	"google.golang.org/grpc/metadata"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/awakari/client-sdk-go/api"
	"github.com/cenkalti/backoff/v4"
	"github.com/zelenin/go-tdlib/client"
	_ "net/http/pprof"
)

func main() {

	// init config and logger
	slog.Info("starting...")
	cfg, err := config.NewConfigFromEnv()
	if err != nil {
		slog.Error("failed to load the config", err)
	}
	opts := slog.HandlerOptions{
		Level: slog.Level(cfg.Log.Level),
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &opts))

	// init the Telegram client
	authorizer := client.ClientAuthorizer()
	go client.CliInteractor(authorizer)
	authorizer.TdlibParameters <- &client.SetTdlibParametersRequest{
		UseTestDc:          false,
		UseSecretChats:     false,
		ApiId:              cfg.Api.Telegram.Id,
		ApiHash:            cfg.Api.Telegram.Hash,
		SystemLanguageCode: "en",
		DeviceModel:        "Awakari",
		SystemVersion:      "1.0.0",
		ApplicationVersion: "1.0.0",
	}
	_, err = client.SetLogVerbosityLevel(&client.SetLogVerbosityLevelRequest{
		NewVerbosityLevel: 2,
	})
	if err != nil {
		panic(err)
	}
	clientTg, err := client.NewClient(authorizer)
	if err != nil {
		panic(err)
	}
	optionValue, err := client.GetOption(&client.GetOptionRequest{
		Name: "version",
	})
	if err != nil {
		panic(err)
	}
	log.Info(fmt.Sprintf("TDLib version: %s", optionValue.(*client.OptionValueString).Value))
	me, err := clientTg.GetMe()
	if err != nil {
		panic(err)
	}
	log.Info(fmt.Sprintf("Me: %s %s [%v]", me.FirstName, me.LastName, me.Usernames))

	// get all chats into the cache - get chat by id won't work w/o this
	_, err = clientTg.GetChats(&client.GetChatsRequest{Limit: 0x100})
	if err != nil {
		panic(err)
	}

	// load the configured chats info
	chats := map[int64]bool{}
	for _, chatId := range cfg.Api.Telegram.Feed.ChatIds {
		var chat *client.Chat
		chat, err = clientTg.GetChat(&client.GetChatRequest{
			ChatId: chatId,
		})
		if err == nil {
			log.Info(fmt.Sprintf("Allowed chat id: %d, title: %s", chatId, chat.Title))
			chats[chatId] = true
		}
		if err != nil {
			log.Error(fmt.Sprintf("Failed to get chat info by id: %d, cause: %s", chatId, err))
		}
	}

	// init the Awakari writer
	var clientAwk api.Client
	clientAwk, err = api.
		NewClientBuilder().
		WriterUri(cfg.Api.Writer.Uri).
		Build()
	if err != nil {
		panic(fmt.Sprintf("failed to initialize the Awakari API client: %s", err))
	}
	log.Info("initialized the Awakari API client")
	defer clientAwk.Close()
	//
	groupIdCtx := metadata.AppendToOutgoingContext(
		context.TODO(),
		"x-awakari-group-id", "producer-telegram",
		"x-awakari-user-id", "producer-telegram",
	)
	w, err := clientAwk.OpenMessagesWriter(groupIdCtx, "producer-telegram")
	if err != nil {
		panic(fmt.Sprintf("failed to open the Awakari events writer: %s", err))
	}
	if err == nil {
		defer w.Close()
		log.Info("opened the Awakari events writer")
	}

	// init handlers
	msgHandler := message.NewHandler(clientTg, chats, w, log)

	// expose the profiling
	go func() {
		_ = http.ListenAndServe("localhost:8080", nil)
	}()

	//
	listener := clientTg.GetListener()
	defer listener.Close()
	h := update.NewHandler(listener, msgHandler)
	b := backoff.NewExponentialBackOff()
	err = backoff.RetryNotify(h.Listen, b, func(err error, d time.Duration) {
		log.Warn(fmt.Sprintf("Failed to handle an update, cause: %s, retrying in: %s...", err, d))
	})
	if err != nil {
		panic(err)
	}

	//
	ch := make(chan os.Signal, 2)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ch
		clientTg.Stop()
		os.Exit(1)
	}()
}

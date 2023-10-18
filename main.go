package main

import (
	"context"
	"fmt"
	"github.com/Arman92/go-tdlib"
	"github.com/awakari/client-sdk-go/api"
	"github.com/awakari/source-telegram/config"
	"github.com/awakari/source-telegram/handler"
	"github.com/awakari/source-telegram/handler/message"
	"github.com/cenkalti/backoff/v4"
	"google.golang.org/grpc/metadata"
	"log/slog"
	"math"
	"os"
	"os/signal"
	"syscall"
	//_ "net/http/pprof"
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
	tdLibConfig := tdlib.Config{
		UseTestDataCenter:  false,
		UseSecretChats:     false,
		APIID:              cfg.Api.Telegram.Id,
		APIHash:            cfg.Api.Telegram.Hash,
		SystemLanguageCode: "en",
		DeviceModel:        "Awakari",
		SystemVersion:      "1.0.0",
		ApplicationVersion: "1.0.0",
	}
	clientTg := tdlib.NewClient(tdLibConfig)
	if err != nil {
		panic(err)
	}

	// Telegram auth
	for {
		currentState, _ := clientTg.Authorize()
		if currentState.GetAuthorizationStateEnum() == tdlib.AuthorizationStateWaitPhoneNumberType {
			_, err = clientTg.SendPhoneNumber(cfg.Api.Telegram.Phone)
			if err != nil {
				log.Error(fmt.Sprintf("Error sending phone number: %s", err))
			}
		} else if currentState.GetAuthorizationStateEnum() == tdlib.AuthorizationStateWaitCodeType {
			fmt.Print("Enter code: ")
			var code string
			fmt.Scanln(&code)
			_, err = clientTg.SendAuthCode(code)
			if err != nil {
				log.Error(fmt.Sprintf("Error sending auth code : %v", err))
			}
		} else if currentState.GetAuthorizationStateEnum() == tdlib.AuthorizationStateWaitPasswordType {
			_, err = clientTg.SendAuthPassword(cfg.Api.Telegram.Password)
			if err != nil {
				log.Error(fmt.Sprintf("Error sending auth password: %v", err))
			}
		} else if currentState.GetAuthorizationStateEnum() == tdlib.AuthorizationStateReadyType {
			log.Info("Authorization done")
			break
		}
	}

	//
	me, err := clientTg.GetMe()
	if err != nil {
		panic(err)
	}
	log.Info(fmt.Sprintf("Me: %s %s [%v]", me.FirstName, me.LastName, me.Username))

	// get all chats into the cache - get chat by id won't work w/o this
	_, err = clientTg.GetChats(tdlib.NewChatListMain(), tdlib.JSONInt64(math.MinInt64), math.MinInt64, 0x100)
	if err != nil {
		panic(err)
	}

	// load the configured chats info
	chats := map[int64]bool{}
	for _, chatId := range cfg.Api.Telegram.Feed.ChatIds {
		var chat *tdlib.Chat
		chat, err = clientTg.GetChat(chatId)
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
		"x-awakari-group-id", "source-telegram",
		"x-awakari-user-id", "source-telegram",
	)
	w, err := clientAwk.OpenMessagesWriter(groupIdCtx, "source-telegram")
	if err != nil {
		panic(fmt.Sprintf("failed to open the Awakari events writer: %s", err))
	}
	if err == nil {
		defer w.Close()
		log.Info("opened the Awakari events writer")
	}

	// expose the profiling
	//go func() {
	//	_ = http.ListenAndServe("localhost:6060", nil)
	//}()

	// init handlers
	msgHandler := message.NewHandler(clientTg, chats, w, log)
	b := backoff.NewExponentialBackOff()
	h := handler.ChatFilterHandler{
		Client:     clientTg,
		Chats:      chats,
		MsgHandler: msgHandler,
		QueueSize:  0x100,
		BackOff:    b,
	}

	//
	go func() {
		err = h.Run()
		if err != nil {
			panic(err)
		}
	}()
	updates := clientTg.GetRawUpdatesChannel(0x100)
	defer close(updates)
	for update := range updates {
		// Show all updates
		fmt.Println(update.Data)
		fmt.Print("\n\n")
	}

	//
	ch := make(chan os.Signal, 2)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ch
		clientTg.DestroyInstance()
		os.Exit(1)
	}()
}

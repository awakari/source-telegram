package main

import (
	"bufio"
	"context"
	"fmt"
	modelAwk "github.com/awakari/client-sdk-go/model"
	apiGrpc "github.com/awakari/source-telegram/api/grpc"
	"github.com/awakari/source-telegram/config"
	"github.com/awakari/source-telegram/handler/message"
	"github.com/awakari/source-telegram/handler/update"
	"github.com/awakari/source-telegram/model"
	"github.com/awakari/source-telegram/storage"
	"github.com/cloudevents/sdk-go/binding/format/protobuf/v2/pb"
	"google.golang.org/grpc/metadata"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/akurilov/go-tdlib/client"
	"github.com/awakari/client-sdk-go/api"
	"github.com/cenkalti/backoff/v4"
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
	authorizer := client.ClientAuthorizer()
	chCode := make(chan string)
	go func() {
		var tgCodeIn *os.File
		tgCodeIn, err = os.OpenFile("tgcodein", os.O_RDONLY, os.ModeNamedPipe)
		if err != nil {
			panic(err)
		}
		defer tgCodeIn.Close()
		tgCodeInReader := bufio.NewReader(tgCodeIn)
		var line string
		line, err = tgCodeInReader.ReadString('\n')
		if err != nil {
			panic(err)
		}
		chCode <- line
	}()
	//
	go client.NonInteractiveCredentialsProvider(authorizer, cfg.Api.Telegram.Phone, cfg.Api.Telegram.Password, chCode)
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
		NewVerbosityLevel: 1,
	})
	if err != nil {
		panic(err)
	}
	//
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
	_, err = clientTg.GetChats(&client.GetChatsRequest{Limit: 1000})
	if err != nil {
		panic(err)
	}

	// init the channel storage
	var stor storage.Storage
	stor, err = storage.NewStorage(context.TODO(), cfg.Db)
	if err != nil {
		panic(err)
	}
	defer stor.Close()

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
	ctxGroupId := metadata.AppendToOutgoingContext(
		context.TODO(),
		"x-awakari-group-id",
		"com.github.awakari.source-telegram",
	)

	// determine the replica index
	replicaNameParts := strings.Split(cfg.Replica.Name, "-")
	if len(replicaNameParts) < 2 {
		panic("unable to parse the replica name: " + cfg.Replica.Name)
	}
	var replicaIndexTmp uint64
	replicaIndexTmp, err = strconv.ParseUint(replicaNameParts[len(replicaNameParts)-1], 10, 16)
	if err != nil {
		panic(err)
	}
	replicaIndex := uint32(replicaIndexTmp)
	log.Info(fmt.Sprintf("Replica: %d/%d", replicaIndex, cfg.Replica.Range))

	// load the configured chats info
	chatWriters := map[int64]modelAwk.Writer[*pb.CloudEvent]{}
	chanFilter := model.ChannelFilter{
		IdDiv: cfg.Replica.Range,
		IdRem: replicaIndex,
	}
	var chans []model.Channel
	chans, err = stor.GetPage(context.TODO(), chanFilter, 1_000_000, "") // it's important to get all at once
	if err != nil {
		panic(err)
	}
	for _, ch := range chans {
		var chat *client.Chat
		chat, err = clientTg.GetChat(&client.GetChatRequest{
			ChatId: ch.Id,
		})
		switch err {
		case nil:
			var link *client.MessageLink
			if chat.LastMessage != nil || chat.LastMessage.ForwardInfo == nil {
				link, err = clientTg.GetMessageLink(&client.GetMessageLinkRequest{
					ChatId:    chat.Id,
					MessageId: chat.LastMessage.Id,
				})
				if err != nil {
					log.Warn(fmt.Sprintf("Last message link: %+v, err: %s", link, err))
				}
			}
			if link != nil && link.Link != "" {
				switch link.IsPublic {
				case true:
					ch.Link = link.Link[:strings.LastIndex(link.Link, "/")] // truncate msg id
				default:
					ch.Link = link.Link // full message link
				}
			}
			if ch.Name == "" || ch.Name != chat.Title {
				ch.Name = chat.Title
			}
			err = stor.Update(context.TODO(), ch)
			if err != nil {
				log.Error(fmt.Sprintf("Failed to update chat %+v title in DB", ch))
			}
			log.Debug(fmt.Sprintf("Selected chat id: %d, title: %s", ch.Id, chat.Title))
			var w modelAwk.Writer[*pb.CloudEvent]
			userId := strconv.FormatInt(ch.Id, 10)
			w, err = clientAwk.OpenMessagesWriter(ctxGroupId, userId)
			switch err {
			case nil:
				chatWriters[ch.Id] = w
			default:
				log.Error(fmt.Sprintf("Failed to open a writer for the chat id: %d, cause: %s", ch.Id, err))
			}
		default:
			log.Error(fmt.Sprintf("Failed to get chat info by id: %d, cause: %s", ch.Id, err))
		}
	}
	defer func() {
		for _, w := range chatWriters {
			_ = w.Close()
		}
	}()

	// init handlers
	msgHandler := message.NewHandler(clientTg, chatWriters, log)

	// expose the profiling
	//go func() {
	//	_ = http.ListenAndServe("localhost:6060", nil)
	//}()

	log.Info(fmt.Sprintf("starting to listen the API @ port #%d...", cfg.Api.Port))
	go apiGrpc.Serve(stor, cfg.Api.Port)

	//
	listener := clientTg.GetListener()
	defer listener.Close()
	h := update.NewHandler(listener, msgHandler)
	b := backoff.NewExponentialBackOff()
	err = backoff.RetryNotify(h.Listen, b, func(err error, d time.Duration) {
		log.Error(fmt.Sprintf("Failed to handle an update, cause: %s, retrying in: %s...", err, d))
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

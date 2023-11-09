package main

import (
	"bufio"
	"context"
	"fmt"
	apiGrpc "github.com/awakari/source-telegram/api/grpc"
	"github.com/awakari/source-telegram/config"
	"github.com/awakari/source-telegram/handler/message"
	"github.com/awakari/source-telegram/handler/update"
	"github.com/awakari/source-telegram/model"
	"github.com/awakari/source-telegram/service"
	"github.com/awakari/source-telegram/storage"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
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

	// init the channel storage
	var stor storage.Storage
	stor, err = storage.NewStorage(context.TODO(), cfg.Db)
	if err != nil {
		panic(err)
	}
	stor = storage.NewStorageLogging(stor, log)
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

	chansJoined := map[int64]model.Channel{}
	chansJoinedLock := &sync.Mutex{}

	svc := service.Service{
		ClientTg:        clientTg,
		Storage:         stor,
		ChansJoined:     chansJoined,
		ChansJoinedLock: chansJoinedLock,
		ReplicaRange:    cfg.Replica.Range,
		ReplicaIndex:    replicaIndex,
		Log:             log,
	}
	go func() {
		b := backoff.NewExponentialBackOff()
		_ = backoff.RetryNotify(svc.RefreshJoinedLoop, b, func(err error, d time.Duration) {
			log.Error(fmt.Sprintf("Failed to refresh joined channels, cause: %s, retrying in: %s...", err, d))
		})
	}()

	// init handlers
	msgHandler := message.NewHandler(clientAwk, clientTg, chansJoined, chansJoinedLock, log)
	defer msgHandler.Close()

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
	defer h.Close()
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

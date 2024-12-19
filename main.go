package main

import (
	"context"
	"fmt"
	apiGrpc "github.com/awakari/source-telegram/api/grpc"
	"github.com/awakari/source-telegram/api/grpc/queue"
	"github.com/awakari/source-telegram/api/http/pub"
	"github.com/awakari/source-telegram/config"
	"github.com/awakari/source-telegram/handler/message"
	"github.com/awakari/source-telegram/handler/update"
	"github.com/awakari/source-telegram/model"
	"github.com/awakari/source-telegram/service"
	"github.com/awakari/source-telegram/storage"
	"github.com/cloudevents/sdk-go/binding/format/protobuf/v2/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/akurilov/go-tdlib/client"
	"github.com/cenkalti/backoff/v4"
	//_ "net/http/pprof"
)

const chanCacheSize = 1_000
const chanCacheTtl = 1 * time.Minute

func main() {

	// init config and logger
	slog.Info("starting...")
	cfg, err := config.NewConfigFromEnv()
	if err != nil {
		slog.Error(fmt.Sprintf("failed to load the config: %s", err))
	}
	opts := slog.HandlerOptions{
		Level: slog.Level(cfg.Log.Level),
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &opts))

	// determine the replica index
	replicaNameParts := strings.Split(cfg.Replica.Name, "-")
	if len(replicaNameParts) < 2 {
		panic("unable to parse the replica name: " + cfg.Replica.Name)
	}
	var replicaIndex int
	replicaIndex, err = strconv.Atoi(replicaNameParts[len(replicaNameParts)-1])
	if err != nil {
		panic(err)
	}
	if replicaIndex < 0 {
		panic(fmt.Sprintf("Negative replica index: %d", replicaIndex))
	}
	log.Info(fmt.Sprintf("Replica: %d", replicaIndex))

	if len(cfg.Api.Telegram.Ids) <= replicaIndex {
		panic("Not enough telegram client ids, decrease the replica count or fix the config")
	}
	if len(cfg.Api.Telegram.Hashes) <= replicaIndex {
		panic("Not enough telegram client hashes, decrease the replica count or fix the config")
	}
	if len(cfg.Api.Telegram.Phones) <= replicaIndex {
		panic("Not enough phone numbers, decrease the replica count or fix the config")
	}

	// init the Telegram client
	authorizer := client.ClientAuthorizer()
	chCode := make(chan string)
	go client.NonInteractiveCredentialsProvider(authorizer, cfg.Api.Telegram.Phones[replicaIndex], cfg.Api.Telegram.Password, chCode)
	authorizer.TdlibParameters <- &client.SetTdlibParametersRequest{
		//
		UseTestDc:          false,
		UseSecretChats:     false,
		ApiId:              cfg.Api.Telegram.Ids[replicaIndex],
		ApiHash:            cfg.Api.Telegram.Hashes[replicaIndex],
		SystemLanguageCode: "en",
		DeviceModel:        "Awakari",
		SystemVersion:      "1.0.0",
		ApplicationVersion: "1.0.0",
		// db opts
		UseFileDatabase:        true,
		UseChatInfoDatabase:    true,
		UseMessageDatabase:     true,
		EnableStorageOptimizer: true,
	}
	_, err = client.SetLogVerbosityLevel(&client.SetLogVerbosityLevelRequest{
		NewVerbosityLevel: 1,
	})
	if err != nil {
		panic(err)
	}
	//
	c := apiGrpc.NewController(chCode)
	log.Info(fmt.Sprintf("starting to listen the API @ port #%d...", cfg.Api.Port))
	go apiGrpc.Serve(c, cfg.Api.Port)
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
	stor = storage.NewLocalCache(stor, chanCacheSize, chanCacheTtl)
	stor = storage.NewStorageLogging(stor, log)
	defer stor.Close()

	chansJoined := map[int64]*model.Channel{}
	chansJoinedLock := &sync.Mutex{}

	tok := cfg.Api.Telegram.Bot.Token
	tokParts := strings.SplitN(tok, ":", 2)
	if len(tokParts) != 2 {
		panic(fmt.Sprintf("invalid telegram bot token: %s", tok))
	}
	var botUserId int64
	botUserId, err = strconv.ParseInt(tokParts[0], 10, 64)
	if err != nil {
		panic(err)
	}

	svc := service.NewService(
		clientTg,
		stor,
		chansJoined,
		chansJoinedLock,
		log,
		replicaIndex,
		botUserId,
		cfg.Db.Table.RefreshInterval,
		cfg.Search.ChanMembersCountMin,
	)
	svc = service.NewServiceLogging(svc, log)
	c.SetService(svc)
	go func() {
		b := backoff.NewExponentialBackOff()
		_ = backoff.RetryNotify(svc.RefreshJoinedLoop, b, func(err error, d time.Duration) {
			log.Error(fmt.Sprintf("Failed to refresh joined channels, cause: %s, retrying in: %s...", err, d))
		})
	}()

	svcPub := pub.NewService(http.DefaultClient, cfg.Api.Writer.Uri, cfg.Api.Token.Internal)
	svcPub = pub.NewLogging(svcPub, log)

	// init handlers
	msgHandler := message.NewHandler(svcPub, clientTg, chansJoined, chansJoinedLock, log, replicaIndex)

	// expose the profiling
	//go func() {
	//	_ = http.ListenAndServe("localhost:6060", nil)
	//}()

	if replicaIndex == cfg.Api.Queue.ReplicaIndex {
		// init queues
		connQueue, err := grpc.NewClient(cfg.Api.Queue.Uri, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			panic(err)
		}
		log.Info("connected to the queue service")
		clientQueue := queue.NewServiceClient(connQueue)
		svcQueue := queue.NewService(clientQueue)
		svcQueue = queue.NewLoggingMiddleware(svcQueue, log)
		err = svcQueue.SetConsumer(context.TODO(), cfg.Api.Queue.InterestsCreated.Name, cfg.Api.Queue.InterestsCreated.Subj)
		if err != nil {
			panic(err)
		}
		log.Info(fmt.Sprintf("initialized the %s queue", cfg.Api.Queue.InterestsCreated.Name))
		go func() {
			err = consumeQueue(
				context.Background(),
				svc,
				svcQueue,
				cfg.Api.Queue.InterestsCreated.Name,
				cfg.Api.Queue.InterestsCreated.Subj,
				cfg.Api.Queue.InterestsCreated.BatchSize,
			)
			if err != nil {
				panic(err)
			}
		}()
		err = svcQueue.SetConsumer(context.TODO(), cfg.Api.Queue.InterestsUpdated.Name, cfg.Api.Queue.InterestsUpdated.Subj)
		if err != nil {
			panic(err)
		}
		log.Info(fmt.Sprintf("initialized the %s queue", cfg.Api.Queue.InterestsUpdated.Name))
		go func() {
			err = consumeQueue(
				context.Background(),
				svc,
				svcQueue,
				cfg.Api.Queue.InterestsUpdated.Name,
				cfg.Api.Queue.InterestsUpdated.Subj,
				cfg.Api.Queue.InterestsUpdated.BatchSize,
			)
			if err != nil {
				panic(err)
			}
		}()
	}

	//
	listener := clientTg.GetListener()
	defer listener.Close()
	h := update.NewHandler(listener, msgHandler, log)
	err = h.Listen(context.Background())
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

func consumeQueue(
	ctx context.Context,
	svc service.Service,
	svcQueue queue.Service,
	name, subj string,
	batchSize uint32,
) (err error) {
	for {
		err = svcQueue.ReceiveMessages(ctx, name, subj, batchSize, func(evts []*pb.CloudEvent) (err error) {
			for _, evt := range evts {
				_ = svc.HandleInterestChange(ctx, evt)
			}
			return
		})
		if err != nil {
			panic(err)
		}
	}
}

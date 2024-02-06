package grpc

import (
	"context"
	"fmt"
	"github.com/awakari/source-telegram/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"log/slog"
	"os"
	"testing"
)

var port uint16 = 50051

var log = slog.Default()

func TestMain(m *testing.M) {
	svc := service.NewServiceMock()
	go func() {
		err := Serve(svc, port)
		if err != nil {
			log.Error("", err)
		}
	}()
	code := m.Run()
	os.Exit(code)
}

func TestServiceClient_Create(t *testing.T) {
	//
	addr := fmt.Sprintf("localhost:%d", port)
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.Nil(t, err)
	client := NewServiceClient(conn)
	//
	cases := map[string]struct {
		ch  *Channel
		err error
	}{
		"missing payload": {
			err: status.Error(codes.InvalidArgument, "channel payload is missing"),
		},
		"ok": {
			ch: &Channel{
				Id:      -123456789,
				GroupId: "group0",
				UserId:  "user0",
				Name:    "channel 0",
				Link:    "https://t.me/channel0",
			},
		},
		"fail": {
			ch: &Channel{
				Id:      -123456789,
				GroupId: "group0",
				UserId:  "user0",
				Name:    "fail",
				Link:    "https://t.me/channel0",
			},
			err: status.Error(codes.Internal, "internal failure"),
		},
		"conflict": {
			ch: &Channel{
				Id:      -123456789,
				GroupId: "group0",
				UserId:  "user0",
				Name:    "conflict",
				Link:    "https://t.me/channel0",
			},
			err: status.Error(codes.AlreadyExists, "channel with the same id is already present"),
		},
	}
	//
	for k, c := range cases {
		t.Run(k, func(t *testing.T) {
			_, err = client.Create(context.TODO(), &CreateRequest{
				Channel: c.ch,
			})
			assert.ErrorIs(t, err, c.err)
		})
	}
}

func TestServiceClient_Read(t *testing.T) {
	//
	addr := fmt.Sprintf("localhost:%d", port)
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.Nil(t, err)
	client := NewServiceClient(conn)
	//
	cases := map[string]struct {
		link string
		ch   *Channel
		err  error
	}{
		"ok": {
			link: "https://t.me/channel0",
			ch: &Channel{
				Id:      -1001801930101,
				GroupId: "group0",
				UserId:  "user0",
				Name:    "channel0",
				Link:    "https://t.me/channel0",
			},
		},
		"fail": {
			link: "fail",
			err:  status.Error(codes.Internal, "internal failure"),
		},
		"missing": {
			link: "missing",
			err:  status.Error(codes.NotFound, "channel not found"),
		},
	}
	//
	for k, c := range cases {
		t.Run(k, func(t *testing.T) {
			var resp *ReadResponse
			resp, err = client.Read(context.TODO(), &ReadRequest{
				Link: c.link,
			})
			if c.err == nil {
				assert.Equal(t, c.ch, resp.Channel)
			}
			assert.ErrorIs(t, err, c.err)
		})
	}
}

func TestServiceClient_Delete(t *testing.T) {
	//
	addr := fmt.Sprintf("localhost:%d", port)
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.Nil(t, err)
	client := NewServiceClient(conn)
	//
	cases := map[string]struct {
		link string
		ch   *Channel
		err  error
	}{
		"ok": {
			link: "https://t.me/channel0",
		},
		"fail": {
			link: "fail",
			err:  status.Error(codes.Internal, "internal failure"),
		},
		"missing": {
			link: "missing",
			err:  status.Error(codes.NotFound, "channel not found"),
		},
	}
	//
	for k, c := range cases {
		t.Run(k, func(t *testing.T) {
			_, err = client.Delete(context.TODO(), &DeleteRequest{
				Link: c.link,
			})
			assert.ErrorIs(t, err, c.err)
		})
	}
}

func TestServiceClient_List(t *testing.T) {
	//
	addr := fmt.Sprintf("localhost:%d", port)
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.Nil(t, err)
	client := NewServiceClient(conn)
	//
	cases := map[string]struct {
		cursor string
		page   []int64
		err    error
	}{
		"basic": {
			page: []int64{
				-1001801930101,
				-1001754252633,
			},
		},
		"end of results": {
			cursor: "channel1",
		},
	}
	//
	for k, c := range cases {
		t.Run(k, func(t *testing.T) {
			var resp *ListResponse
			resp, err = client.List(context.TODO(), &ListRequest{Cursor: c.cursor, Limit: 10})
			assert.ErrorIs(t, err, c.err)
			if c.page != nil {
				assert.Equal(t, len(c.page), len(resp.Page))
			}
		})
	}
}
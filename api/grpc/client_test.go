package grpc

import (
	"context"
	"fmt"
	"github.com/awakari/source-telegram/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"log/slog"
	"os"
	"testing"
)

var port uint16 = 50051

var log = slog.Default()

func TestMain(m *testing.M) {
	stor := storage.NewStorageMock()
	go func() {
		err := Serve(stor, port)
		if err != nil {
			log.Error("", err)
		}
	}()
	code := m.Run()
	os.Exit(code)
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

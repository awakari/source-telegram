package grpc

import (
	"fmt"
	"github.com/awakari/source-telegram/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"net"
)

func Serve(svc service.Service, port uint16, chCode chan string, replicaIdx uint32) (err error) {
	srv := grpc.NewServer()
	c := NewController(svc, chCode, replicaIdx)
	RegisterServiceServer(srv, c)
	reflection.Register(srv)
	grpc_health_v1.RegisterHealthServer(srv, health.NewServer())
	conn, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err == nil {
		err = srv.Serve(conn)
	}
	return
}

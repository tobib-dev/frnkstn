package main

import (
	"charm.land/log/v2"
	"github.com/tobib-dev/frnkstn/internal/rpclog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func newGRPCClient(target string) (*grpc.ClientConn, error) {
	return grpc.NewClient(
		target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(rpclog.Handler{
			Message: "client request completed",
			Log: func(message string, fields ...any) {
				log.Info(message, fields...)
			},
		}),
	)
}

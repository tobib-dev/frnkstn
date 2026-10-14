package rpclog

import (
	"context"

	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
)

type Handler struct {
	Message string
	Log     func(string, ...any)
}

type methodKey struct{}

func (h Handler) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	return context.WithValue(ctx, methodKey{}, info.FullMethodName)
}

func (h Handler) HandleRPC(ctx context.Context, rpcStats stats.RPCStats) {
	end, ok := rpcStats.(*stats.End)
	if !ok {
		return
	}
	h.Log(h.Message,
		"method", ctx.Value(methodKey{}),
		"duration_ms", end.EndTime.Sub(end.BeginTime).Seconds()*1000,
		"status", status.Code(end.Error).String(),
	)
}

func (Handler) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context { return ctx }
func (Handler) HandleConn(context.Context, stats.ConnStats)                       {}

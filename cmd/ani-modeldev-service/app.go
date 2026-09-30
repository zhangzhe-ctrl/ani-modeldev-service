package main

import (
	"context"
	"log/slog"
	"time"

	kratos "github.com/go-kratos/kratos/v3"
	kratosgrpc "github.com/go-kratos/kratos/v3/transport/grpc"
	kratoshttp "github.com/go-kratos/kratos/v3/transport/http"

	conf "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/conf/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/server"
)

func buildApp(bc *conf.Bootstrap, logger *slog.Logger) (*kratos.App, error) {
	if err := bc.Validate(); err != nil {
		return nil, err
	}
	// The full training/observation/publication chain is not assembled yet.
	// Durable command delivery alone cannot grant overall business readiness.
	readiness := server.NewReadiness()
	observability, err := server.NewObservability(Name, Version, readiness)
	if err != nil {
		return nil, err
	}
	middlewares := observability.ServerMiddleware(logger)
	var grpcServer *kratosgrpc.Server
	cleanup := func() {}
	if bc.Command == nil {
		grpcServer = server.NewGRPCServer(bc.Server.Grpc, middlewares...)
	} else {
		grpcServer, cleanup, err = buildCommandServer(bc.Server.Grpc, bc.Command, middlewares...)
		if err != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = observability.Shutdown(ctx)
			return nil, err
		}
	}
	adminServer := server.NewAdminServer(bc.Server.Admin, readiness, observability.Gatherer(), middlewares...)
	return newApp(logger, grpcServer, adminServer, readiness, observability, bc.Server.ShutdownTimeout.AsDuration(), cleanup), nil
}

func newApp(
	logger *slog.Logger,
	grpcServer *kratosgrpc.Server,
	adminServer *kratoshttp.Server,
	readiness *server.Readiness,
	observability *server.Observability,
	stopTimeout time.Duration,
	cleanup func(),
) *kratos.App {
	return kratos.New(
		kratos.ID(id),
		kratos.Name(Name),
		kratos.Version(Version),
		kratos.Logger(logger),
		kratos.Server(grpcServer, adminServer),
		kratos.BeforeStop(func(context.Context) error {
			readiness.Set(false)
			return nil
		}),
		kratos.AfterStop(func(ctx context.Context) error {
			cleanup()
			return observability.Shutdown(ctx)
		}),
		kratos.StopTimeout(stopTimeout),
	)
}

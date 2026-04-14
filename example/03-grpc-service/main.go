// Package main demonstrates a gRPC service using forge with JWT auth.
//
// File layout:
//
//	main.go                           — wire & start
//	proto/user/v1/user.proto          — protobuf definitions (buf generate to regenerate)
//	gen/user/v1/                      — generated code
//	internal/service/user.go          — gRPC service implementation
//
// Methods (same operations as 02-http-api):
//
//	Login        — returns JWT, no auth required
//	ListUsers    — paginated list, auth required
//	GetUser      — get by ID, auth required
//	CreateUser   — create user, auth required
//	UpdateUser   — update user, auth required
//	DeleteUser   — delete user, auth required
//
// Test in browser:
//
//	grpcui -plaintext localhost:<port>
package main

import (
	"context"
	"net"

	"github.com/pemako/forge/app"
	"github.com/pemako/forge/auth"
	"github.com/pemako/forge/config"
	"github.com/pemako/forge/logger"
	grpcserver "github.com/pemako/forge/server/grpc"

	userv1 "github.com/pemako/forge/example/03-grpc-service/gen/user/v1"
	"github.com/pemako/forge/example/03-grpc-service/internal/service"
)

func main() {
	cfg, err := config.Load(
		config.WithFile("config.yaml"),
		config.WithEnv("APP"),
	)
	if err != nil {
		panic(err)
	}

	log, err := logger.New(logger.WithConfig(cfg))
	if err != nil {
		panic(err)
	}
	logger.SetDefault(log)

	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}

	authOpts := []auth.Option{
		auth.WithConfig(cfg),
		auth.WithClaims(func() auth.Claims { return &service.UserClaims{} }),
	}

	srv, err := grpcserver.New(
		grpcserver.WithListener(ln),
		grpcserver.WithReflection(true),
		grpcserver.WithGRPCUI(":0"), // random port, logs URL at startup
		grpcserver.WithUnaryInterceptors(
			auth.OptionalUnaryAuth(authOpts...),
		),
	)
	if err != nil {
		panic(err)
	}

	userSvc := service.NewUserService(authOpts)
	userv1.RegisterUserServiceServer(srv.Server(), userSvc)

	_, port, _ := net.SplitHostPort(ln.Addr().String())

	application := app.New(
		app.WithName("grpc-service"),
		app.WithVersion("0.1.0"),
		app.WithServer(srv),
		app.WithBeforeStart(func(ctx context.Context) error {
			logger.Info(ctx, "grpc-service started",
				logger.F("addr", "localhost:"+port),
			)
			return nil
		}),
	)

	if err := application.Run(); err != nil {
		logger.Error(context.Background(), "application exited with error",
			logger.F("error", err),
		)
	}
}

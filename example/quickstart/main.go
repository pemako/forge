package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/pemako/mk/app"
	"github.com/pemako/mk/config"
	"github.com/pemako/mk/errors"
	"github.com/pemako/mk/logger"
	mw "github.com/pemako/mk/middleware/http"
	httpserver "github.com/pemako/mk/server/http"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	log, err := logger.New(logger.WithConfig(cfg))
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger: %v\n", err)
		os.Exit(1)
	}

	srv, err := httpserver.New(
		httpserver.WithConfig(cfg),
		httpserver.WithLogger(log),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "http server: %v\n", err)
		os.Exit(1)
	}

	r := srv.Engine()
	r.Use(
		mw.RequestID(),
		mw.Logger(log),
		mw.Recovery(),
		mw.ErrorHandler(),
	)

	r.GET("/hello", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "hello"})
	})

	r.GET("/error-demo", func(c *gin.Context) {
		// Correct pattern: use c.Error with a CodedError; ErrorHandler middleware
		// converts it to the appropriate HTTP status and JSON body.
		c.Error(errors.Define(400, "DEMO_ERROR", "demo error")) //nolint:errcheck
	})

	if err := app.New(
		app.WithName("quickstart"),
		app.WithLogger(log),
		app.WithServer(srv),
	).Run(); err != nil {
		log.Error(context.Background(), "run failed", logger.F("err", err))
	}
}

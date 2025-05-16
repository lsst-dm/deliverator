package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/lsst-dm/s3nd/conf"
	"github.com/lsst-dm/s3nd/handler"
	"github.com/lsst-dm/s3nd/version"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	logger.Info("starting s3nd", "version", version.Version)

	conf := conf.NewConf()

	handler := handler.NewHandler(&conf)
	http.Handle("/", handler)

	addr := fmt.Sprintf("%s:%d", *conf.Host, *conf.Port)
	logger.Info("listening", "address", addr)

	s := &http.Server{
		Addr:         addr,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
	}
	err := s.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		logger.Info("server closed")
	} else if err != nil {
		logger.Error("failed to start server", "error", err)
		os.Exit(1)
	}
}

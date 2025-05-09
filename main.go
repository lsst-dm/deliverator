package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/lsst-dm/s3nd/conf"
	"github.com/lsst-dm/s3nd/handler"
)

func main() {
	conf := conf.NewConf()

	handler := handler.NewHandler(&conf)
	http.Handle("/", handler)

	addr := fmt.Sprintf("%s:%d", *conf.Host, *conf.Port)
	log.Println("Listening on", addr)

	s := &http.Server{
		Addr:         addr,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
	}
	err := s.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		log.Printf("server closed\n")
	} else if err != nil {
		log.Fatalf("error starting server: %s\n", err)
	}
}

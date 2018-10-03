package main

import (
	"context"
	"flag"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	slsHTTP "github.com/egtann/sls/http"
	"github.com/egtann/up"
)

func main() {
	rand.Seed(time.Now().UnixNano())
	confFilePath := flag.String("c", "sls.conf", "config filepath")
	flag.Parse()
	log := &logger{}
	conf, err := loadConfig(*confFilePath)
	if err != nil {
		log.Fatal(err)
	}
	version, err := up.GetCalculatedChecksum("checksum")
	if err != nil {
		log.Fatal(err)
	}

	// TODO - load an error reporter and pass into ServeNewMux
	service, err := slsHTTP.NewService(log, conf.Dir, conf.APIKey, version)
	if err != nil {
		log.Fatal(err)
	}
	defer service.Shutdown()

	// Periodically check if the file needs to be split and delete old
	// files outside the retention period
	go service.EnforceRetentionPolicy(conf.RetainFor)

	srv := &http.Server{
		Addr:           ":" + conf.Port,
		Handler:        service.Mux,
		ReadTimeout:    10 * time.Minute,
		WriteTimeout:   0,
		MaxHeaderBytes: 1 << 20,
	}
	go func() {
		if err = srv.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()
	log.Printf("listening on %s\n", conf.Port)
	gracefulRestart(srv, time.Second)
}

// gracefulRestart listens for an interrupt or terminate signal. When either is
// received, it stops accepting new connections and allows all existing
// connections up to the timeout duration to complete. If connections do not
// shut down in time, sls exits with 1.
func gracefulRestart(srv *http.Server, timeout time.Duration) {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Println("failed to shutdown server gracefully", err)
		os.Exit(1)
	}
	log.Println("shut down")
}

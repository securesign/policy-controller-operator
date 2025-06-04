package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/securesign/policy-controller-operator/cmd/webhook"
)

func main() {
	var (
		tlsKey  = flag.String("tls-key", "/tmp/k8s-webhook-server/serving-certs/tls.key", "Path to TLS key")
		tlsCert = flag.String("tls-cert", "/tmp/k8s-webhook-server/serving-certs/tls.crt", "Path to TLS certificate")
		addr    = flag.String("addr", ":9443", "Listen address")
	)
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/validate", webhook.ServeValidate)

	srv := &http.Server{
		Addr:    *addr,
		Handler: mux,
	}

	idleConnsClosed := make(chan struct{})
	go func() {
		log.Info().Msgf("HTTPS server listening on %s", *addr)
		if err := srv.ListenAndServeTLS(*tlsCert, *tlsKey); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("failed to start admission webhook server")
		}
		close(idleConnsClosed)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Info().Msg("Shutdown signal received, shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("server forced to shutdown")
	}

	<-idleConnsClosed
	log.Info().Msg("Server shutdown complete")
}

package main

import (
	"flag"
	"net/http"

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

	log.Info().Msgf("HTTPS server listening on %s", *addr)
	if err := srv.ListenAndServeTLS(*tlsCert, *tlsKey); err != nil && err != http.ErrServerClosed {
		log.Fatal().Err(err).Msg("failed to start admission webhook server")
	}
}

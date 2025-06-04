package main

import (
	"context"
	"flag"
	"os"

	"github.com/securesign/policy-controller-operator/cmd/internal/constants"
	rhtas_webhook "github.com/securesign/policy-controller-operator/cmd/internal/webhook"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

func init() {
	log.SetLogger(zap.New())
}

func main() {
	var (
		certDir = flag.String("cert-dir", "/tmp/k8s-webhook-server/serving-certs", "CertDir is the directory that contains the server key and certificate. Defaults to <temp-dir>/k8s-webhook-server/serving-certs.")
		port    = flag.Int("port", 9443, "Port is the port number that the server will serve. It will be defaulted to 9443 if unspecified.")
	)
	flag.Parse()

	entryLog := log.Log.WithName("entrypoint")

	// Setup a Manager
	entryLog.Info("setting up manager")
	mgr, err := manager.New(config.GetConfigOrDie(), manager.Options{
		WebhookServer: webhook.NewServer(webhook.Options{
			Port:    *port,
			CertDir: *certDir,
		}),
	})
	if err != nil {
		entryLog.Error(err, "unable to set up overall controller manager")
		os.Exit(1)
	}

	policyControllerGVK := schema.GroupVersionKind{
		Group:   constants.PolicyControllerGroup,
		Version: constants.PolicyControllerVersion,
		Kind:    constants.PolicyControllerKind,
	}
	mgr.GetScheme().AddKnownTypeWithName(policyControllerGVK, &unstructured.Unstructured{})

	policyController := &unstructured.Unstructured{}
	policyController.SetGroupVersionKind(policyControllerGVK)
	if err := builder.WebhookManagedBy(mgr).
		For(policyController).
		WithValidator(&rhtas_webhook.PolicyControllerValidator{}).
		WithCustomPath("/validate").
		Complete(); err != nil {
		entryLog.Error(err, "unable to create webhook for PolicyController")
		os.Exit(1)
	}

	entryLog.Info("starting manager")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		entryLog.Error(err, "unable to run manager")
		os.Exit(1)
	}

	<-idleConnsClosed
	log.Info().Msg("Server shutdown complete")
}

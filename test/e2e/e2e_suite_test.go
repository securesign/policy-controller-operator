package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	e2e_utils "github.com/securesign/policy-controller-operator/test/e2e/utils"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/e2e-framework/klient/conf"
)

var (
	k8sClient client.Client
	ctx       context.Context
	scheme    = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
}

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)
	log.SetLogger(GinkgoLogr)
	SetDefaultEventuallyTimeout(time.Duration(3) * time.Minute)
	EnforceDefaultTimeoutsWhenUsingContexts()
	RunSpecs(t, "Policy Controller E2E Suite")

	format.MaxLength = 0
}

var _ = SynchronizedBeforeSuite(func() []byte {
	kubeconfig := conf.ResolveKubeConfigFile()
	data, err := os.ReadFile(kubeconfig)
	Expect(err).NotTo(HaveOccurred())
	return data
}, func(data []byte) {
	restCfg, err := clientcmd.RESTConfigFromKubeConfig(data)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(restCfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())

	ctx = context.Background()

	fmt.Println(">>> Running tests with the following parameters:")
	fmt.Printf("  %-22s %s\n", "RHTAS Install Namespace:", e2e_utils.RhtasInstallNamespace())
	fmt.Printf("  %-24s %s\n", "TUF URL:", e2e_utils.TufUrl())
	fmt.Printf("  %-24s %s\n", "TSA URL:", e2e_utils.TsaUrl())
	fmt.Printf("  %-24s %s\n", "Rekor URL:", e2e_utils.RekorUrl())
	fmt.Printf("  %-24s %s\n", "Fulcio URL:", e2e_utils.FulcioUrl())
	fmt.Printf("  %-24s %s\n", "OIDC Issuer URL:", e2e_utils.OidcIssuerUrl())
	fmt.Printf("  %-24s %s\n", "OIDC Issuer Subject:", e2e_utils.OidcIssuerSubject())
	fmt.Printf("  %-24s %s\n", "Inject CA:", e2e_utils.InjectCA())
})

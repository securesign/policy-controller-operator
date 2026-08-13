//go:build fips

package e2e_fips

import (
	. "github.com/onsi/ginkgo/v2"
	e2e_utils "github.com/securesign/policy-controller-operator/test/utils"
)

var _ = Describe("Policy Controller FIPS Strict Mode (fips140=only)", Ordered, func() {
	It("verifies the cluster kernel has FIPS enabled", func(ctx SpecContext) {
		pod, containerName := e2e_utils.GetDeploymentPod(ctx, k8sClient, e2e_utils.InstallNamespace, e2e_utils.DeploymentName)
		e2e_utils.VerifyFIPSKernel(pod.Name, containerName, pod.Namespace)
	})

	It("verifies the policy-controller webhook is running in FIPS mode", func(ctx SpecContext) {
		pod, containerName := e2e_utils.GetDeploymentPod(ctx, k8sClient, e2e_utils.InstallNamespace, e2e_utils.DeploymentName)
		e2e_utils.VerifyFIPSGoNative(pod.Name, containerName, pod.Namespace)
	})

	It("verifies the admission-webhook-controller is running in FIPS mode", func(ctx SpecContext) {
		operatorDep := e2e_utils.FindOperatorDeployment(ctx, k8sClient)
		pod, _ := e2e_utils.GetDeploymentPod(ctx, k8sClient, operatorDep.Namespace, operatorDep.Name)
		e2e_utils.VerifyFIPSGoNative(pod.Name, "admission-webhook-controller", pod.Namespace)
	})
})

package constants

const (
	PolicyControllerGroup      = "rhtas.charts.redhat.com"
	PolicyControllerVersion    = "v1alpha1"
	PolicyControllerAPIVersion = PolicyControllerGroup + "/" + PolicyControllerVersion
	PolicyControllerResource   = "policycontrollers"
	PolicyControllerKind       = "PolicyController"
	PolicyControllerInstallNs  = "policy-controller-operator"
)

package e2e_utils

import (
	"os/exec"
	"strings"

	"github.com/onsi/ginkgo/v2/dsl/core"
)

func Execute(command string, args ...string) error {
	cmd := exec.Command(command, args...)
	cmd.Stderr = core.GinkgoWriter
	cmd.Stdout = core.GinkgoWriter
	return cmd.Run()
}

func ExecuteWithInput(input string, command string, args ...string) error {
	cmd := exec.Command(command, args...)
	cmd.Stdin = strings.NewReader(input)
	cmd.Stderr = core.GinkgoWriter
	cmd.Stdout = core.GinkgoWriter
	return cmd.Run()
}

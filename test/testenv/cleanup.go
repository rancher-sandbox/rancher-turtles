/*
Copyright © 2023 - 2024 SUSE LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package testenv

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/cluster-api/test/framework"
)

type CleanupTestClusterInput struct {
	SetupTestClusterResult
	SkipCleanup    bool
	ArtifactFolder string
}

func CleanupTestCluster(ctx context.Context, input CleanupTestClusterInput) {
	Expect(ctx).NotTo(BeNil(), "ctx is required for CleanupTestCluster")
	Expect(input.SetupTestClusterResult).ToNot(BeNil(), "SetupTestClusterResult is required for CleanupTestCluster")
	Expect(input.ArtifactFolder).ToNot(BeEmpty(), "ArtifactFolder is required for CleanupTestCluster")

	By("Dumping artifacts from the bootstrap cluster")
	dumpBootstrapCluster(ctx, input.BootstrapClusterProxy, input.ArtifactFolder)

	if input.SkipCleanup {
		return
	}

	By("Tearing down the management cluster")
	if input.BootstrapClusterProxy != nil {
		input.BootstrapClusterProxy.Dispose(ctx)
	}
	if input.BootstrapClusterProvider != nil {
		input.BootstrapClusterProvider.Dispose(ctx)
	}
}

var secrets = []string{
	"NGROK_AUTHTOKEN",
	"NGROK_API_KEY",
	"RANCHER_HOSTNAME",
	"RANCHER_PASSWORD",
	"CAPA_ENCODED_CREDS",
	"AZURE_SUBSCRIPTION_ID",
	"AZURE_CLIENT_ID",
	"AZURE_CLIENT_SECRET",
	"AZURE_TENANT_ID",
}

func CollectArtifacts(ctx context.Context, kubeconfigPath, name string, args ...string) error {
	if kubeconfigPath == "" {
		return fmt.Errorf("Unable to collect artifacts: kubeconfig path is empty")
	}

	aargs := append([]string{"crust-gather", "collect", "-v", "ERROR", "--kubeconfig", kubeconfigPath, "-f", name}, args...)
	for _, secret := range secrets {
		aargs = append(aargs, "-s", secret)
	}

	cmd := exec.Command("kubectl", aargs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.WaitDelay = time.Minute

	fmt.Printf("Running kubectl %s\n", strings.Join(aargs, " "))
	err := cmd.Run()
	fmt.Printf("stderr:\n%s\n", string(stderr.Bytes()))
	fmt.Printf("stdout:\n%s\n", string(stdout.Bytes()))
	return err
}

func dumpBootstrapCluster(ctx context.Context, bootstrapClusterProxy framework.ClusterProxy, artifactFolder string) {
	if bootstrapClusterProxy == nil {
		return
	}

	err := CollectArtifacts(ctx, bootstrapClusterProxy.GetKubeconfigPath(), path.Join(artifactFolder, bootstrapClusterProxy.GetName(), "bootstrap"))
	if err != nil {
		fmt.Printf("Failed to artifacts for the bootstrap cluster: %v\n", err)
		return
	}
}

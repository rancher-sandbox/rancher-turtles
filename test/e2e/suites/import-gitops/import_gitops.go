//go:build e2e
// +build e2e

/*
Copyright 2023 SUSE.

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

package import_gitops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	provisioningv1 "github.com/rancher-sandbox/rancher-turtles/internal/rancher/provisioning/v1"
	"github.com/rancher-sandbox/rancher-turtles/test/e2e"
	turtlesframework "github.com/rancher-sandbox/rancher-turtles/test/framework"
	turtlesnaming "github.com/rancher-sandbox/rancher-turtles/util/naming"
)

type CreateUsingGitOpsSpecInput struct {
	E2EConfig             *clusterctl.E2EConfig
	BootstrapClusterProxy framework.ClusterProxy
	ClusterctlConfigPath  string
	ArtifactFolder        string
	RancherServerURL      string

	ClusterctlBinaryPath string
	ClusterTemplate      []byte
	ClusterName          string

	CAPIClusterCreateWaitName string
	DeleteClusterWaitName     string

	// ControlPlaneMachineCount defines the number of control plane machines to be added to the workload cluster.
	// If not specified, 1 will be used.
	ControlPlaneMachineCount *int

	// WorkerMachineCount defines number of worker machines to be added to the workload cluster.
	// If not specified, 1 will be used.
	WorkerMachineCount *int

	// OverrideKubernetesVersion if specified will override the Kubernetes version used in the cluster template.
	OverrideKubernetesVersion string

	GitAddr           string
	GitAuthSecretName string

	SkipCleanup      bool
	SkipDeletionTest bool

	LabelNamespace bool
}

// CreateUsingGitOpsSpec implements a spec that will create a cluster via Fleet and test that it
// automatically imports into Rancher Manager.
func CreateUsingGitOpsSpec(ctx context.Context, inputGetter func() CreateUsingGitOpsSpecInput) {
	var (
		specName              = "creategitops"
		input                 CreateUsingGitOpsSpecInput
		namespace             *corev1.Namespace
		repoName              string
		cancelWatches         context.CancelFunc
		capiCluster           *types.NamespacedName
		rancherKubeconfig     *turtlesframework.RancherGetClusterKubeconfigResult
		rancherConnectRes     *turtlesframework.RunCommandResult
		capiClusterCreateWait []interface{}
		deleteClusterWait     []interface{}
	)

	BeforeEach(func() {
		Expect(ctx).NotTo(BeNil(), "ctx is required for %s spec", specName)
		input = inputGetter()
		Expect(input.E2EConfig).ToNot(BeNil(), "Invalid argument. input.E2EConfig can't be nil when calling %s spec", specName)
		Expect(input.BootstrapClusterProxy).ToNot(BeNil(), "Invalid argument. input.BootstrapClusterProxy can't be nil when calling %s spec", specName)
		Expect(input.ClusterctlConfigPath).To(BeAnExistingFile(), "Invalid argument. input.ClusterctlConfigPath must be an existing file when calling %s spec", specName)
		Expect(os.MkdirAll(input.ArtifactFolder, 0750)).To(Succeed(), "Invalid argument. input.ArtifactFolder can't be created for %s spec", specName)

		Expect(input.E2EConfig.Variables).To(HaveKey(e2e.KubernetesVersionVar))
		namespace, cancelWatches = e2e.SetupSpecNamespace(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder)
		repoName = e2e.CreateRepoName(specName)

		capiClusterCreateWait = input.E2EConfig.GetIntervals(input.BootstrapClusterProxy.GetName(), input.CAPIClusterCreateWaitName)
		Expect(capiClusterCreateWait).ToNot(BeNil(), "Failed to get wait intervals %s", input.CAPIClusterCreateWaitName)

		deleteClusterWait = input.E2EConfig.GetIntervals(input.BootstrapClusterProxy.GetName(), input.DeleteClusterWaitName)
		Expect(capiClusterCreateWait).ToNot(BeNil(), "Failed to get wait intervals %s", input.CAPIClusterCreateWaitName)

		capiCluster = &types.NamespacedName{
			Namespace: namespace.Name,
			Name:      input.ClusterName,
		}

		rancherKubeconfig = new(turtlesframework.RancherGetClusterKubeconfigResult)
		rancherConnectRes = new(turtlesframework.RunCommandResult)

		komega.SetClient(input.BootstrapClusterProxy.GetClient())
		komega.SetContext(ctx)
	})

	It("Should import a cluster using gitops", func() {
		controlPlaneMachineCount := 1
		if input.ControlPlaneMachineCount != nil {
			controlPlaneMachineCount = *input.ControlPlaneMachineCount
		}

		workerMachineCount := 1
		if input.WorkerMachineCount != nil {
			workerMachineCount = *input.WorkerMachineCount
		}

		if input.LabelNamespace {
			turtlesframework.AddLabelsToNamespace(ctx, turtlesframework.AddLabelsToNamespaceInput{
				ClusterProxy: input.BootstrapClusterProxy,
				Name:         namespace.Name,
				Labels: map[string]string{
					"cluster-api.cattle.io/rancher-auto-import": "true",
				},
			})
		}

		By("Create Git repository")

		repoCloneAddr := turtlesframework.GiteaCreateRepo(ctx, turtlesframework.GiteaCreateRepoInput{
			ServerAddr: input.GitAddr,
			RepoName:   repoName,
			Username:   input.E2EConfig.GetVariable(e2e.GiteaUserNameVar),
			Password:   input.E2EConfig.GetVariable(e2e.GiteaUserPasswordVar),
		})
		repoDir := turtlesframework.GitCloneRepo(ctx, turtlesframework.GitCloneRepoInput{
			Address:  repoCloneAddr,
			Username: input.E2EConfig.GetVariable(e2e.GiteaUserNameVar),
			Password: input.E2EConfig.GetVariable(e2e.GiteaUserPasswordVar),
		})

		By("Create fleet repository structure")

		clustersDir := filepath.Join(repoDir, "clusters")
		os.MkdirAll(clustersDir, os.ModePerm)

		additionalVariables := map[string]string{
			"CLUSTER_NAME":                input.ClusterName,
			"WORKER_MACHINE_COUNT":        strconv.Itoa(workerMachineCount),
			"CONTROL_PLANE_MACHINE_COUNT": strconv.Itoa(controlPlaneMachineCount),
		}

		if input.OverrideKubernetesVersion != "" {
			additionalVariables["KUBERNETES_VERSION"] = input.OverrideKubernetesVersion
		}

		clusterPath := filepath.Join(clustersDir, fmt.Sprintf("%s.yaml", input.ClusterName))
		Expect(turtlesframework.ApplyFromTemplate(ctx, turtlesframework.ApplyFromTemplateInput{
			Getter:                        input.E2EConfig.GetVariable,
			Template:                      input.ClusterTemplate,
			OutputFilePath:                clusterPath,
			AddtionalEnvironmentVariables: additionalVariables,
		})).To(Succeed())

		fleetPath := filepath.Join(clustersDir, "fleet.yaml")
		turtlesframework.FleetCreateFleetFile(ctx, turtlesframework.FleetCreateFleetFileInput{
			Namespace: namespace.Name,
			FilePath:  fleetPath,
		})

		By("Committing changes to fleet repo and pushing")

		turtlesframework.GitCommitAndPush(ctx, turtlesframework.GitCommitAndPushInput{
			CloneLocation: repoDir,
			Username:      input.E2EConfig.GetVariable(e2e.GiteaUserNameVar),
			Password:      input.E2EConfig.GetVariable(e2e.GiteaUserPasswordVar),
			CommitMessage: "ci: add clusters bundle",
		})

		By("Applying GitRepo")

		turtlesframework.FleetCreateGitRepo(ctx, turtlesframework.FleetCreateGitRepoInput{
			Name:             repoName,
			Namespace:        turtlesframework.FleetLocalNamespace,
			Branch:           turtlesframework.DefaultBranchName,
			Repo:             repoCloneAddr,
			FleetGeneration:  1,
			Paths:            []string{"clusters"},
			ClientSecretName: input.GitAuthSecretName,
			ClusterProxy:     input.BootstrapClusterProxy,
		})

		By("Waiting for the CAPI cluster to appear")
		capiCluster := &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace.Name,
			Name:      input.ClusterName,
		}}
		Eventually(
			komega.Get(capiCluster),
			input.E2EConfig.GetIntervals(input.BootstrapClusterProxy.GetName(), "wait-rancher")...).
			Should(Succeed(), "Failed to apply CAPI cluster definition to cluster via Fleet")

		By("Waiting for cluster control plane to be Ready")
		Eventually(komega.Object(capiCluster), capiClusterCreateWait...).Should(HaveField("Status.ControlPlaneReady", BeTrue()))

		By("Waiting for the CAPI cluster to be connectable")
		Eventually(func() error {
			remoteClient := input.BootstrapClusterProxy.GetWorkloadCluster(ctx, capiCluster.Namespace, capiCluster.Name).GetClient()
			namespaces := &corev1.NamespaceList{}

			return remoteClient.List(ctx, namespaces)
		}, capiClusterCreateWait...).Should(Succeed(), "Failed to connect to workload cluster using CAPI kubeconfig")

		By("Waiting for the rancher cluster record to appear")
		rancherCluster := &provisioningv1.Cluster{ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace.Name,
			Name:      turtlesnaming.Name(capiCluster.Name).ToRancherName(),
		}}
		Eventually(komega.Get(rancherCluster), input.E2EConfig.GetIntervals(input.BootstrapClusterProxy.GetName(), "wait-rancher")...).Should(Succeed())

		By("Waiting for the rancher cluster to have a deployed agent")
		Eventually(komega.Object(rancherCluster), input.E2EConfig.GetIntervals(input.BootstrapClusterProxy.GetName(), "wait-rancher")...).Should(HaveField("Status.AgentDeployed", BeTrue()))

		By("Waiting for the rancher cluster to be ready")
		Eventually(komega.Object(rancherCluster), input.E2EConfig.GetIntervals(input.BootstrapClusterProxy.GetName(), "wait-rancher")...).Should(HaveField("Status.Ready", BeTrue()))

		By("Waiting for the CAPI cluster to be connectable using Rancher kubeconfig")
		turtlesframework.RancherGetClusterKubeconfig(ctx, turtlesframework.RancherGetClusterKubeconfigInput{
			Getter:           input.BootstrapClusterProxy.GetClient(),
			SecretName:       fmt.Sprintf("%s-capi-kubeconfig", capiCluster.Name),
			Namespace:        capiCluster.Namespace,
			RancherServerURL: input.RancherServerURL,
			WriteToTempFile:  true,
		}, rancherKubeconfig)

		turtlesframework.RunCommand(ctx, turtlesframework.RunCommandInput{
			Command: "kubectl",
			Args: []string{
				"--kubeconfig",
				rancherKubeconfig.TempFilePath,
				"get",
				"nodes",
				"--insecure-skip-tls-verify",
			},
		}, rancherConnectRes)
		Expect(rancherConnectRes.Error).NotTo(HaveOccurred(), "Failed getting nodes with Rancher Kubeconfig")
		Expect(rancherConnectRes.ExitCode).To(Equal(0), "Getting nodes return non-zero exit code")

		By("Deleting GitRepo from Rancher")
		turtlesframework.FleetDeleteGitRepo(ctx, turtlesframework.FleetDeleteGitRepoInput{
			Name:         repoName,
			Namespace:    turtlesframework.FleetLocalNamespace,
			ClusterProxy: input.BootstrapClusterProxy,
		})

		By("Waiting for the rancher cluster record to be removed")
		Eventually(komega.Get(rancherCluster), deleteClusterWait...).Should(MatchError(ContainSubstring("not found")), "Rancher cluster should be deleted")

	})

	AfterEach(func() {
		e2e.DumpSpecResourcesAndCleanup(ctx, specName, input.BootstrapClusterProxy, input.ArtifactFolder, namespace, cancelWatches, capiCluster, input.E2EConfig.GetIntervals, input.SkipCleanup)
	})
}

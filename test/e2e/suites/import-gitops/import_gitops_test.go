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
	. "github.com/onsi/ginkgo/v2"

	"github.com/rancher-sandbox/rancher-turtles/test/e2e"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
	. "sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	_ "embed"
)

var (
	//go:embed cluster-templates/docker-kubeadm.yaml
	dockerKubeadm []byte

	//go:embed cluster-templates/aws-eks-mmp.yaml
	awsEKSMMP []byte

	//go:embed cluster-templates/azure-aks-mmp.yaml
	azureAKSMMP []byte

	//go:embed cluster-templates/docker-rke2.yaml
	rke2Docker []byte
)

var _ = Describe("[Docker] [Kubeadm] Create and delete CAPI cluster functionality should work with namespace auto-import", Label(e2e.ShortTestLabel, e2e.FullTestLabel), func() {

	BeforeEach(func() {
		SetClient(setupClusterResult.BootstrapClusterProxy.GetClient())
		SetContext(ctx)
	})

	CreateUsingGitOpsSpec(ctx, func() CreateUsingGitOpsSpecInput {
		return CreateUsingGitOpsSpecInput{
			E2EConfig:                 e2eConfig,
			BootstrapClusterProxy:     setupClusterResult.BootstrapClusterProxy,
			ClusterctlConfigPath:      flagVals.ConfigPath,
			ClusterctlBinaryPath:      flagVals.ClusterctlBinaryPath,
			ArtifactFolder:            flagVals.ArtifactFolder,
			ClusterTemplate:           dockerKubeadm,
			ClusterName:               "cluster1",
			ControlPlaneMachineCount:  ptr.To[int](1),
			WorkerMachineCount:        ptr.To[int](1),
			GitAddr:                   giteaResult.GitAddress,
			GitAuthSecretName:         e2e.AuthSecretName,
			SkipCleanup:               false,
			SkipDeletionTest:          false,
			LabelNamespace:            true,
			RancherServerURL:          hostName,
			CAPIClusterCreateWaitName: "wait-rancher",
			DeleteClusterWaitName:     "wait-controllers",
		}
	})
})

var _ = Describe("[AWS] [EKS] Create and delete CAPI cluster functionality should work with namespace auto-import", Label(e2e.FullTestLabel), func() {

	BeforeEach(func() {
		komega.SetClient(setupClusterResult.BootstrapClusterProxy.GetClient())
		komega.SetContext(ctx)
	})

	CreateUsingGitOpsSpec(ctx, func() CreateUsingGitOpsSpecInput {
		return CreateUsingGitOpsSpecInput{
			E2EConfig:                 e2eConfig,
			BootstrapClusterProxy:     setupClusterResult.BootstrapClusterProxy,
			ClusterctlConfigPath:      flagVals.ConfigPath,
			ClusterctlBinaryPath:      flagVals.ClusterctlBinaryPath,
			ArtifactFolder:            flagVals.ArtifactFolder,
			ClusterTemplate:           awsEKSMMP,
			ClusterName:               "cluster2",
			ControlPlaneMachineCount:  ptr.To[int](1),
			WorkerMachineCount:        ptr.To[int](1),
			GitAddr:                   giteaResult.GitAddress,
			GitAuthSecretName:         e2e.AuthSecretName,
			SkipCleanup:               false,
			SkipDeletionTest:          false,
			LabelNamespace:            true,
			RancherServerURL:          hostName,
			CAPIClusterCreateWaitName: "wait-capa-create-cluster",
			DeleteClusterWaitName:     "wait-eks-delete",
		}
	})
})

var _ = Describe("[Azure] [AKS] Create and delete CAPI cluster functionality should work with namespace auto-import", Label(e2e.FullTestLabel), func() {

	BeforeEach(func() {
		SetClient(setupClusterResult.BootstrapClusterProxy.GetClient())
		SetContext(ctx)
	})

	CreateUsingGitOpsSpec(ctx, func() CreateUsingGitOpsSpecInput {
		return CreateUsingGitOpsSpecInput{
			E2EConfig:                 e2eConfig,
			BootstrapClusterProxy:     setupClusterResult.BootstrapClusterProxy,
			ClusterctlConfigPath:      flagVals.ConfigPath,
			ArtifactFolder:            flagVals.ArtifactFolder,
			ClusterTemplate:           azureAKSMMP,
			ClusterName:               "cluster-azure-aks",
			ControlPlaneMachineCount:  ptr.To[int](1),
			WorkerMachineCount:        ptr.To[int](1),
			GitAddr:                   giteaResult.GitAddress,
			GitAuthSecretName:         e2e.AuthSecretName,
			SkipCleanup:               false,
			SkipDeletionTest:          false,
			LabelNamespace:            true,
			RancherServerURL:          hostName,
			CAPIClusterCreateWaitName: "wait-capz-create-cluster",
			DeleteClusterWaitName:     "wait-aks-delete",
		}
	})
})

var _ = FDescribe("[Docker] [RKE2] Create and delete CAPI cluster functionality should work with namespace auto-import", Label(e2e.FullTestLabel), func() {

	BeforeEach(func() {
		SetClient(setupClusterResult.BootstrapClusterProxy.GetClient())
		SetContext(ctx)
	})

	CreateUsingGitOpsSpec(ctx, func() CreateUsingGitOpsSpecInput {
		return CreateUsingGitOpsSpecInput{
			E2EConfig:                 e2eConfig,
			BootstrapClusterProxy:     setupClusterResult.BootstrapClusterProxy,
			ClusterctlConfigPath:      flagVals.ConfigPath,
			ArtifactFolder:            flagVals.ArtifactFolder,
			ClusterTemplate:           rke2Docker,
			ClusterName:               "cluster-rke2-docker",
			OverrideKubernetesVersion: "v1.28.2",
			ControlPlaneMachineCount:  ptr.To[int](1),
			WorkerMachineCount:        ptr.To[int](1),
			GitAddr:                   giteaResult.GitAddress,
			GitAuthSecretName:         e2e.AuthSecretName,
			SkipCleanup:               false,
			SkipDeletionTest:          false,
			LabelNamespace:            true,
			RancherServerURL:          hostName,
			CAPIClusterCreateWaitName: "wait-rke2-docker",
			DeleteClusterWaitName:     "wait-rke2-docker",
		}
	})
})

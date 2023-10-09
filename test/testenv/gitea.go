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

package testenv

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	opframework "sigs.k8s.io/cluster-api-operator/test/framework"
	"sigs.k8s.io/cluster-api/test/framework"

	turtlesframework "github.com/rancher-sandbox/rancher-turtles/test/framework"
)

type DeployGiteaInput struct {
	BootstrapClusterProxy framework.ClusterProxy
	HelmBinaryPath        string
	ChartRepoName         string
	ChartRepoURL          string
	ChartName             string
	ChartVersion          string
	ValuesFilePath        string
	Values                map[string]string
	RolloutWaitInterval   []interface{}
	ServiceWaitInterval   []interface{}
	Username              string
	Password              string
	AuthSecretName        string
	UseExistingCluster    bool
}

type DeployGiteaResult struct {
	GitAddress string
}

func DeployGitea(ctx context.Context, input DeployGiteaInput) *DeployGiteaResult {
	Expect(ctx).NotTo(BeNil(), "ctx is required for DeployGitea")
	Expect(input.BootstrapClusterProxy).ToNot(BeNil(), "BootstrapClusterProxy is required for DeployGitea")
	Expect(input.HelmBinaryPath).ToNot(BeEmpty(), "HelmBinaryPath is required for DeployGitea")
	Expect(input.ChartRepoName).ToNot(BeEmpty(), "ChartRepoName is required for DeployGitea")
	Expect(input.ChartRepoURL).ToNot(BeEmpty(), "ChartRepoURL is required for DeployGitea")
	Expect(input.ChartName).ToNot(BeEmpty(), "ChartName is required for DeployGitea")
	Expect(input.ChartVersion).ToNot(BeEmpty(), "Chartversion is required for DeployGitea")
	Expect(input.RolloutWaitInterval).ToNot(BeNil(), "RolloutWaitInterval is required for DeployGitea")
	Expect(input.ServiceWaitInterval).ToNot(BeNil(), "ServiceWaitInterval is required for DeployGitea")

	if input.UseExistingCluster {
		addr := turtlesframework.GetNodeAddress(ctx, turtlesframework.GetNodeAddressInput{
			Lister:       input.BootstrapClusterProxy.GetClient(),
			NodeIndex:    0,
			AddressIndex: 0,
		})
		port := turtlesframework.GetServicePortByName(ctx, turtlesframework.GetServicePortByNameInput{
			GetLister:        input.BootstrapClusterProxy.GetClient(),
			ServiceName:      "gitea-http",
			ServiceNamespace: "default",
			PortName:         "http",
		}, input.ServiceWaitInterval...)
		Expect(port.NodePort).ToNot(Equal(0), "Node port for Gitea service is not set")
		return &DeployGiteaResult{
			GitAddress: fmt.Sprintf("http://%s:%d", addr, port.NodePort),
		}
	}

	if input.Username != "" {
		Expect(input.Password).ToNot(BeEmpty(), "Password is required for DeployGitea if a username is supplied")
		Expect(input.AuthSecretName).ToNot(BeEmpty(), "AuthSecretName is required for DeployGitea if a username is supplied")
	}

	result := &DeployGiteaResult{}

	By("Installing gitea chart")
	addChart := &opframework.HelmChart{
		BinaryPath:      input.HelmBinaryPath,
		Name:            input.ChartRepoName,
		Path:            input.ChartRepoURL,
		Commands:        opframework.Commands(opframework.Repo, opframework.Add),
		AdditionalFlags: opframework.Flags("--force-update"),
		Kubeconfig:      input.BootstrapClusterProxy.GetKubeconfigPath(),
	}
	_, err := addChart.Run(nil)
	Expect(err).ToNot(HaveOccurred())

	updateChart := &opframework.HelmChart{
		BinaryPath: input.HelmBinaryPath,
		Commands:   opframework.Commands(opframework.Repo, opframework.Update),
		Kubeconfig: input.BootstrapClusterProxy.GetKubeconfigPath(),
	}
	_, err = updateChart.Run(nil)
	Expect(err).ToNot(HaveOccurred())

	flags := opframework.Flags(
		"--version", input.ChartVersion,
		"--create-namespace",
		"--wait",
	)
	if input.ValuesFilePath != "" {
		flags = append(flags, "-f", input.ValuesFilePath)
	}
	chart := &opframework.HelmChart{
		BinaryPath:      input.HelmBinaryPath,
		Path:            fmt.Sprintf("%s/%s", input.ChartRepoName, input.ChartName),
		Name:            "gitea",
		Kubeconfig:      input.BootstrapClusterProxy.GetKubeconfigPath(),
		AdditionalFlags: flags,
	}

	// Gitea values can be found gitea_values.yaml file as well. For a list of the values
	// available look here: https://gitea.com/gitea/helm-chart/src/branch/main/values.yaml
	_, err = chart.Run(input.Values)
	Expect(err).ToNot(HaveOccurred())

	By("Waiting for gitea rollout")
	framework.WaitForDeploymentsAvailable(ctx, framework.WaitForDeploymentsAvailableInput{
		Getter:     input.BootstrapClusterProxy.GetClient(),
		Deployment: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "gitea", Namespace: "default"}},
	}, input.RolloutWaitInterval...)

	By("Get Git server config")
	addr := turtlesframework.GetNodeAddress(ctx, turtlesframework.GetNodeAddressInput{
		Lister:       input.BootstrapClusterProxy.GetClient(),
		NodeIndex:    0,
		AddressIndex: 0,
	})
	port := turtlesframework.GetServicePortByName(ctx, turtlesframework.GetServicePortByNameInput{
		GetLister:        input.BootstrapClusterProxy.GetClient(),
		ServiceName:      "gitea-http",
		ServiceNamespace: "default",
		PortName:         "http",
	}, input.ServiceWaitInterval...)
	Expect(port.NodePort).ToNot(Equal(0), "Node port for Gitea service is not set")
	result.GitAddress = fmt.Sprintf("http://%s:%d", addr, port.NodePort)

	if input.Username == "" {
		By("No gitea username, skipping creation of auth secret")
		return result
	}

	turtlesframework.CreateSecret(ctx, turtlesframework.CreateSecretInput{
		Creator:   input.BootstrapClusterProxy.GetClient(),
		Name:      input.AuthSecretName,
		Namespace: turtlesframework.FleetLocalNamespace,
		Type:      corev1.SecretTypeBasicAuth,
		Data: map[string]string{
			"username": input.Username,
			"password": input.Password,
		},
	})

	return result
}

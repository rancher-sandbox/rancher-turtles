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
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	opframework "sigs.k8s.io/cluster-api-operator/test/framework"
	"sigs.k8s.io/cluster-api/test/framework"

	turtlesframework "github.com/rancher-sandbox/rancher-turtles/test/framework"
)

type DeployRancherTurtlesInput struct {
	BootstrapClusterProxy        framework.ClusterProxy
	HelmBinaryPath               string
	ChartPath                    string
	CAPIProvidersSecretYAML      []byte
	CAPIProvidersYAML            []byte
	Namespace                    string
	Image                        string
	Tag                          string
	WaitDeploymentsReadyInterval []interface{}
	AdditionalValues             map[string]string
}

func DeployRancherTurtles(ctx context.Context, input DeployRancherTurtlesInput) {

	Expect(ctx).NotTo(BeNil(), "ctx is required for DeployRancherTurtles")
	Expect(input.BootstrapClusterProxy).ToNot(BeNil(), "BootstrapClusterProxy is required for DeployRancherTurtles")
	Expect(input.CAPIProvidersYAML).ToNot(BeNil(), "CAPIProvidersYAML is required for DeployRancherTurtles")
	Expect(input.ChartPath).ToNot(BeEmpty(), "ChartPath is required for DeployRancherTurtles")
	Expect(input.HelmBinaryPath).ToNot(BeEmpty(), "HelmBinaryPath is required for DeployRancherTurtles")
	Expect(input.Image).ToNot(BeEmpty(), "Image is required for DeployRancherTurtles")
	Expect(input.Tag).ToNot(BeEmpty(), "Tag is required for DeployRancherTurtles")
	Expect(input.WaitDeploymentsReadyInterval).ToNot(BeNil(), "WaitDeploymentsReadyInterval is required for DeployRancherTurtles")

	namespace := input.Namespace
	if namespace == "" {
		namespace = turtlesframework.DefaultRancherTurtlesNamespace
	}

	if input.CAPIProvidersSecretYAML != nil {
		By("Adding CAPI variables secret")
		Expect(input.BootstrapClusterProxy.Apply(ctx, input.CAPIProvidersSecretYAML)).To(Succeed())
	}

	By("Installing rancher-turtles chart")
	chart := &opframework.HelmChart{
		BinaryPath: input.HelmBinaryPath,
		Path:       input.ChartPath,
		Name:       "rancher-turtles",
		Kubeconfig: input.BootstrapClusterProxy.GetKubeconfigPath(),
		AdditionalFlags: opframework.Flags(
			"--dependency-update",
			"-n", namespace,
			"--create-namespace", "--wait"),
	}

	values := map[string]string{
		"rancherTurtles.image":                               input.Image,
		"rancherTurtles.imageVersion":                        input.Tag,
		"rancherTurtles.tag":                                 input.Tag,
		"rancherTurtles.managerArguments[0]":                 "--insecure-skip-verify=true",
		"cluster-api-operator.cluster-api.configSecret.name": "variables",
	}

	for name, val := range input.AdditionalValues {
		values[name] = val
	}

	_, err := chart.Run(values)
	Expect(err).ToNot(HaveOccurred())

	//TODO: this can probably be covered by the Operator helper

	By("Adding CAPI infrastructure providers")
	Expect(input.BootstrapClusterProxy.Apply(ctx, input.CAPIProvidersYAML)).To(Succeed())

	By("Waiting for CAPI deployment to be available")
	framework.WaitForDeploymentsAvailable(ctx, framework.WaitForDeploymentsAvailableInput{
		Getter: input.BootstrapClusterProxy.GetClient(),
		Deployment: &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "capi-controller-manager",
				Namespace: "capi-system",
			}},
	}, input.WaitDeploymentsReadyInterval...)

	By("Waiting for CAPI kubeadm bootstrap deployment to be available")
	framework.WaitForDeploymentsAvailable(ctx, framework.WaitForDeploymentsAvailableInput{
		Getter: input.BootstrapClusterProxy.GetClient(),
		Deployment: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
			Name:      "capi-kubeadm-bootstrap-controller-manager",
			Namespace: "capi-kubeadm-bootstrap-system",
		}},
	}, input.WaitDeploymentsReadyInterval...)

	By("Waiting for CAPI kubeadm control plane deployment to be available")
	framework.WaitForDeploymentsAvailable(ctx, framework.WaitForDeploymentsAvailableInput{
		Getter: input.BootstrapClusterProxy.GetClient(),
		Deployment: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
			Name:      "capi-kubeadm-control-plane-controller-manager",
			Namespace: "capi-kubeadm-control-plane-system",
		}},
	}, input.WaitDeploymentsReadyInterval...)

	By("Waiting for CAPI docker provider deployment to be available")
	framework.WaitForDeploymentsAvailable(ctx, framework.WaitForDeploymentsAvailableInput{
		Getter: input.BootstrapClusterProxy.GetClient(),
		Deployment: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
			Name:      "capd-controller-manager",
			Namespace: "capd-system",
		}},
	}, input.WaitDeploymentsReadyInterval...)

	By("Waiting for CAPI RKE2 bootstrap deployment to be available")
	framework.WaitForDeploymentsAvailable(ctx, framework.WaitForDeploymentsAvailableInput{
		Getter: input.BootstrapClusterProxy.GetClient(),
		Deployment: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
			Name:      "rke2-bootstrap-controller-manager",
			Namespace: "rke2-bootstrap-system",
		}},
	}, input.WaitDeploymentsReadyInterval...)

	By("Waiting for CAPI RKE2 control plane deployment to be available")
	framework.WaitForDeploymentsAvailable(ctx, framework.WaitForDeploymentsAvailableInput{
		Getter: input.BootstrapClusterProxy.GetClient(),
		Deployment: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
			Name:      "rke2-control-plane-controller-manager",
			Namespace: "rke2-control-plane-system",
		}},
	}, input.WaitDeploymentsReadyInterval...)
}

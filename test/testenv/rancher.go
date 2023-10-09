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
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/drone/envsubst/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	opframework "sigs.k8s.io/cluster-api-operator/test/framework"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
)

type DeployRancherInput struct {
	BootstrapClusterProxy  framework.ClusterProxy
	HelmBinaryPath         string
	RancherChartRepoName   string
	RancherChartURL        string
	RancherChartPath       string
	RancherVersion         string
	RancherImageTag        string
	RancherNamespace       string
	RancherHost            string
	RancherPassword        string
	RancherFeatures        string
	RancherSettingsPatch   []byte
	RancherWaitInterval    []interface{}
	ControllerWaitInterval []interface{}
	IsolatedMode           bool
	RancherIngressConfig   []byte
	RancherServicePatch    []byte
	Development            bool
	UseExistingCluster     bool
}

func DeployRancher(ctx context.Context, input DeployRancherInput) {
	if input.UseExistingCluster {
		return
	}

	Expect(ctx).NotTo(BeNil(), "ctx is required for DeployRancher")
	Expect(input.BootstrapClusterProxy).ToNot(BeNil(), "BootstrapClusterProxy is required for DeployRancher")
	Expect(input.HelmBinaryPath).ToNot(BeEmpty(), "HelmBinaryPath is required for DeployRancher")
	Expect(input.RancherChartRepoName).ToNot(BeEmpty(), "RancherChartRepoName is required for DeployRancher")
	Expect(input.RancherChartURL).ToNot(BeEmpty(), "RancherChartURL is required for DeployRancher")
	Expect(input.RancherChartPath).ToNot(BeEmpty(), "RancherChartPath is required for DeployRancher")
	Expect(input.RancherNamespace).ToNot(BeEmpty(), "RancherNamespace is required for DeployRancher")
	Expect(input.RancherHost).ToNot(BeEmpty(), "RancherHost is required for DeployRancher")
	Expect(input.RancherPassword).ToNot(BeEmpty(), "RancherPassword is required for DeployRancher")
	Expect(input.RancherWaitInterval).ToNot(BeNil(), "RancherWaitInterval is required for DeployRancher")
	Expect(input.ControllerWaitInterval).ToNot(BeNil(), "ControllerWaitInterval is required for DeployRancher")

	if input.RancherVersion == "" && input.RancherImageTag == "" {
		Fail("RancherVersion or RancherImageTag is required")
	}
	if input.RancherVersion != "" && input.RancherImageTag != "" {
		Fail("Only one of RancherVersion or RancherImageTag cen be used")
	}

	By("Adding Rancher chart repo")
	addChart := &opframework.HelmChart{
		BinaryPath:      input.HelmBinaryPath,
		Name:            input.RancherChartRepoName,
		Path:            input.RancherChartURL,
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

	By("Installing Rancher")
	installFlags := opframework.Flags(
		"--namespace", input.RancherNamespace,
		"--create-namespace",
		"--wait",
	)
	if input.RancherVersion != "" {
		installFlags = append(installFlags, "--version", input.RancherVersion)
	}
	if input.Development {
		installFlags = append(installFlags, "--devel")
	}

	chart := &opframework.HelmChart{
		BinaryPath:      input.HelmBinaryPath,
		Path:            input.RancherChartPath,
		Name:            "rancher",
		Kubeconfig:      input.BootstrapClusterProxy.GetKubeconfigPath(),
		AdditionalFlags: installFlags,
	}
	values := map[string]string{
		"bootstrapPassword":         input.RancherPassword,
		"global.cattle.psp.enabled": "false",
		"hostname":                  input.RancherHost,
		"replicas":                  "1",
	}
	if input.RancherFeatures != "" {
		values["features"] = input.RancherFeatures
	}
	if input.RancherImageTag != "" {
		values["rancherImageTag"] = input.RancherImageTag
	}

	_, err = chart.Run(values)
	Expect(err).ToNot(HaveOccurred())

	if len(input.RancherSettingsPatch) > 0 {
		By("Updating rancher settings")
		settingPatch, err := envsubst.Eval(string(input.RancherSettingsPatch), func(s string) string {
			switch s {
			case "RANCHER_HOSTNAME":
				return input.RancherHost
			default:
				return os.Getenv(s)
			}
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(input.BootstrapClusterProxy.Apply(ctx, []byte(settingPatch))).To(Succeed())
	}

	if !input.IsolatedMode {
		By("Setting up ingress")

		ingress, err := envsubst.Eval(string(input.RancherIngressConfig), os.Getenv)
		Expect(err).ToNot(HaveOccurred())
		Expect(input.BootstrapClusterProxy.Apply(ctx, []byte(ingress))).To(Succeed())

		By("Updating rancher svc")
		Expect(input.BootstrapClusterProxy.Apply(ctx, input.RancherServicePatch, "--server-side")).To(Succeed())
	}

	By("Waiting for rancher webhook rollout")
	framework.WaitForDeploymentsAvailable(ctx, framework.WaitForDeploymentsAvailableInput{
		Getter:     input.BootstrapClusterProxy.GetClient(),
		Deployment: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "rancher-webhook", Namespace: input.RancherNamespace}},
	}, input.RancherWaitInterval...)

	// hack: fleet controller needs to be restarted first to pickup config change with a valid API url.
	framework.WaitForDeploymentsAvailable(ctx, framework.WaitForDeploymentsAvailableInput{
		Getter:     input.BootstrapClusterProxy.GetClient(),
		Deployment: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "fleet-controller", Namespace: "cattle-fleet-system"}},
	}, input.ControllerWaitInterval...)

	By("Bouncing the fleet")
	Eventually(func() error {
		return input.BootstrapClusterProxy.GetClient().DeleteAllOf(ctx, &corev1.Pod{}, client.InNamespace("cattle-fleet-system"), client.MatchingLabels{"app": "fleet-controller"})
	}, input.ControllerWaitInterval...).ShouldNot(HaveOccurred())
}

type RancherDeployIngressInput struct {
	BootstrapClusterProxy    framework.ClusterProxy
	HelmBinaryPath           string
	IsolatedMode             bool
	NginxIngress             []byte
	NginxIngressNamespace    string
	IngressWaitInterval      []interface{}
	NgrokApiKey              string
	NgrokAuthToken           string
	NgrokPath                string
	NgrokRepoName            string
	NgrokRepoURL             string
	DefaultIngressClassPatch []byte
	UseExistingCluster       bool
}

func RancherDeployIngress(ctx context.Context, input RancherDeployIngressInput) {

	Expect(ctx).NotTo(BeNil(), "ctx is required for RancherDeployIngress")
	Expect(input.BootstrapClusterProxy).ToNot(BeNil(), "BootstrapClusterProxy is required for RancherDeployIngress")
	if input.IsolatedMode {
		Expect(input.NginxIngress).ToNot(BeEmpty(), "NginxIngress is required when running in isolated mode")
		Expect(input.NginxIngressNamespace).ToNot(BeEmpty(), "NginxIngressNamespace is required when running in isolated mode")
		Expect(input.IngressWaitInterval).ToNot(BeNil(), "IngressWaitInterval is required when running in isolated mode")
	} else {
		Expect(input.NgrokApiKey).ToNot(BeEmpty(), "NgrokApiKey is required when not running in isolated mode")
		Expect(input.NgrokAuthToken).ToNot(BeEmpty(), "NgrokAuthToken is required when not running in isolated mode")
		Expect(input.NgrokPath).ToNot(BeEmpty(), "NgrokPath is required when not running in isolated mode")
		Expect(input.NgrokRepoName).ToNot(BeEmpty(), "NgrokRepoName is required when not running in isolated mode")
		Expect(input.NgrokRepoURL).ToNot(BeEmpty(), "NgrokRepoURL is required when not running in isolated mode")
	}

	komega.SetClient(input.BootstrapClusterProxy.GetClient())
	komega.SetContext(ctx)

	if input.UseExistingCluster {
		return
	}

	if input.IsolatedMode {
		By("Deploying nginx ingress")
		Expect(input.BootstrapClusterProxy.Apply(ctx, []byte(input.NginxIngress))).To(Succeed())

		By("Getting nginx ingress deployment")
		ngixDeployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "ingress-nginx-controller", Namespace: input.NginxIngressNamespace}}
		Eventually(
			komega.Get(ngixDeployment),
			input.IngressWaitInterval...,
		).Should(Succeed(), "Failed to get nginx ingress controller")

		By("Waiting for ingress-nginx-controller deployment to be available")
		Eventually(komega.Object(ngixDeployment), input.IngressWaitInterval...).Should(HaveField("Status.AvailableReplicas", Equal(int32(1))))

		return
	}

	By("Setting up ngrok-ingress-controller")
	addChart := &opframework.HelmChart{
		BinaryPath:      input.HelmBinaryPath,
		Name:            input.NgrokRepoName,
		Path:            input.NgrokRepoURL,
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

	installChart := &opframework.HelmChart{
		BinaryPath:      input.HelmBinaryPath,
		Name:            input.NgrokRepoName,
		Path:            input.NgrokPath,
		Kubeconfig:      input.BootstrapClusterProxy.GetKubeconfigPath(),
		Wait:            true,
		AdditionalFlags: opframework.Flags("--timeout", "5m"),
	}
	_, err = installChart.Run(map[string]string{
		"credentials.apiKey":    input.NgrokApiKey,
		"credentials.authtoken": input.NgrokAuthToken,
	})
	Expect(err).ToNot(HaveOccurred())

	By("Setting up default ingress class")
	Expect(input.BootstrapClusterProxy.Apply(ctx, input.DefaultIngressClassPatch, "--server-side")).To(Succeed())

}

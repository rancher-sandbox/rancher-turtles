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
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/klog/v2"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/rancher-sandbox/rancher-turtles/test/e2e"
	turtlesframework "github.com/rancher-sandbox/rancher-turtles/test/framework"
	"github.com/rancher-sandbox/rancher-turtles/test/testenv"
)

// Test suite flags.
var (
	flagVals *e2e.FlagValues
)

// Test suite global vars.
var (
	// e2eConfig to be used for this test, read from configPath.
	e2eConfig *clusterctl.E2EConfig

	// clusterctlConfigPath to be used for this test, created by generating a clusterctl local repository
	// with the providers specified in the configPath.
	clusterctlConfigPath string

	// hostName is the host name for the Rancher Manager server.
	hostName string

	ctx = context.Background()

	setupClusterResult *testenv.SetupTestClusterResult
	giteaResult        *testenv.DeployGiteaResult
)

func init() {
	flagVals = &e2e.FlagValues{}
	e2e.InitFlags(flagVals)
}

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)

	ctrl.SetLogger(klog.Background())

	RunSpecs(t, "rancher-turtles-e2e-import-gitops")
}

var _ = BeforeSuite(func() {
	Expect(flagVals.ConfigPath).To(BeAnExistingFile(), "Invalid test suite argument. e2e.config should be an existing file.")
	Expect(os.MkdirAll(flagVals.ArtifactFolder, 0755)).To(Succeed(), "Invalid test suite argument. Can't create e2e.artifacts-folder %q", flagVals.ArtifactFolder)
	Expect(flagVals.HelmBinaryPath).To(BeAnExistingFile(), "Invalid test suite argument. helm-binary-path should be an existing file.")
	Expect(flagVals.ChartPath).To(BeAnExistingFile(), "Invalid test suite argument. chart-path should be an existing file.")

	By(fmt.Sprintf("Loading the e2e test configuration from %q", flagVals.ConfigPath))
	e2eConfig = e2e.LoadE2EConfig(flagVals.ConfigPath)

	By(fmt.Sprintf("Creating a clusterctl config into %q", flagVals.ArtifactFolder))
	clusterctlConfigPath = e2e.CreateClusterctlLocalRepository(ctx, e2eConfig, filepath.Join(flagVals.ArtifactFolder, "repository"))

	hostName = e2eConfig.GetVariable(e2e.RancherHostnameVar)

	setupClusterResult = testenv.SetupTestCluster(ctx, testenv.SetupTestClusterInput{
		UseExistingCluster:   flagVals.UseExistingCluster,
		E2EConfig:            e2eConfig,
		ClusterctlConfigPath: clusterctlConfigPath,
		Scheme:               e2e.InitScheme(),
		ArtifactFolder:       flagVals.ArtifactFolder,
		Hostname:             hostName,
		KubernetesVersion:    e2eConfig.GetVariable(e2e.KubernetesVersionVar),
		IsolatedMode:         flagVals.IsolatedMode,
		HelmBinaryPath:       flagVals.HelmBinaryPath,
	})

	if flagVals.IsolatedMode {
		hostName = setupClusterResult.IsolatedHostName
	}

	Expect(e2eConfig.Images).To(HaveLen(1))
	imageNameTag := strings.Split(e2eConfig.Images[0].Name, ":")
	Expect(imageNameTag).To(HaveLen(2))

	testenv.DeployRancherTurtles(ctx, testenv.DeployRancherTurtlesInput{
		BootstrapClusterProxy:        setupClusterResult.BootstrapClusterProxy,
		UseExistingCluster:           flagVals.UseExistingCluster,
		HelmBinaryPath:               flagVals.HelmBinaryPath,
		ChartPath:                    flagVals.ChartPath,
		CAPIProvidersSecretYAML:      e2e.CapiProvidersSecret,
		CAPIProvidersYAML:            e2e.CapiProviders,
		Namespace:                    turtlesframework.DefaultRancherTurtlesNamespace,
		Image:                        imageNameTag[0],
		Tag:                          imageNameTag[1],
		WaitDeploymentsReadyInterval: e2eConfig.GetIntervals(setupClusterResult.BootstrapClusterProxy.GetName(), "wait-controllers"),
	})

	if Label(e2e.FullTestLabel).MatchesLabelFilter(GinkgoLabelFilter()) {
		By("Running full tests, deploying additional infrastructure providers")
		awsCreds := e2eConfig.GetVariable(e2e.CapaEncodedCredentialsVar)
		Expect(awsCreds).ToNot(BeEmpty(), "AWS creds required for full test")

		testenv.CAPIOperatorDeployProvider(ctx, testenv.CAPIOperatorDeployProviderInput{
			BootstrapClusterProxy: setupClusterResult.BootstrapClusterProxy,
			CAPIProvidersSecretsYAML: [][]byte{
				e2e.AWSProviderSecret,
				e2e.AzureProviderSecret,
				e2e.AzureIdentitySecret,
			},
			CAPIProvidersYAML: e2e.FullProviders,
			TemplateData: map[string]string{
				"AWSEncodedCredentials": e2eConfig.GetVariable(e2e.CapaEncodedCredentialsVar),
			},
			WaitDeploymentsReadyInterval: e2eConfig.GetIntervals(setupClusterResult.BootstrapClusterProxy.GetName(), "wait-controllers"),
			WaitForDeployments: []testenv.NamespaceName{
				{
					Name:      "capa-controller-manager",
					Namespace: "capa-system",
				},
				{
					Name:      "capz-controller-manager",
					Namespace: "capz-system",
				},
				{
					Name:      "rke2-bootstrap-controller-manager",
					Namespace: "rke2-bootstrap-system",
				},
				{
					Name:      "rke2-control-plane-controller-manager",
					Namespace: "rke2-control-plane-system",
				},
			},
		})
	}

	testenv.RancherDeployIngress(ctx, testenv.RancherDeployIngressInput{
		UseExistingCluster:       flagVals.UseExistingCluster,
		BootstrapClusterProxy:    setupClusterResult.BootstrapClusterProxy,
		HelmBinaryPath:           flagVals.HelmBinaryPath,
		IsolatedMode:             flagVals.IsolatedMode,
		NginxIngress:             e2e.NginxIngress,
		NginxIngressNamespace:    e2e.NginxIngressNamespace,
		IngressWaitInterval:      e2eConfig.GetIntervals(setupClusterResult.BootstrapClusterProxy.GetName(), "wait-rancher"),
		NgrokApiKey:              e2eConfig.GetVariable(e2e.NgrokApiKeyVar),
		NgrokAuthToken:           e2eConfig.GetVariable(e2e.NgrokAuthTokenVar),
		NgrokPath:                e2eConfig.GetVariable(e2e.NgrokPathVar),
		NgrokRepoName:            e2eConfig.GetVariable(e2e.NgrokRepoNameVar),
		NgrokRepoURL:             e2eConfig.GetVariable(e2e.NgrokUrlVar),
		DefaultIngressClassPatch: e2e.IngressClassPatch,
	})

	testenv.DeployRancher(ctx, testenv.DeployRancherInput{
		UseExistingCluster:     flagVals.UseExistingCluster,
		BootstrapClusterProxy:  setupClusterResult.BootstrapClusterProxy,
		HelmBinaryPath:         flagVals.HelmBinaryPath,
		RancherChartRepoName:   e2eConfig.GetVariable(e2e.RancherRepoNameVar),
		RancherChartURL:        e2eConfig.GetVariable(e2e.RancherUrlVar),
		RancherChartPath:       e2eConfig.GetVariable(e2e.RancherPathVar),
		RancherVersion:         e2eConfig.GetVariable(e2e.RancherVersionVar),
		RancherHost:            hostName,
		RancherNamespace:       e2e.RancherNamespace,
		RancherPassword:        e2eConfig.GetVariable(e2e.RancherPasswordVar),
		RancherFeatures:        e2eConfig.GetVariable(e2e.RancherFeaturesVar),
		RancherSettingsPatch:   e2e.RancherSettingPatch,
		RancherWaitInterval:    e2eConfig.GetIntervals(setupClusterResult.BootstrapClusterProxy.GetName(), "wait-rancher"),
		ControllerWaitInterval: e2eConfig.GetIntervals(setupClusterResult.BootstrapClusterProxy.GetName(), "wait-controllers"),
		IsolatedMode:           flagVals.IsolatedMode,
		RancherIngressConfig:   e2e.IngressConfig,
		RancherServicePatch:    e2e.RancherServicePatch,
	})

	giteaResult = testenv.DeployGitea(ctx, testenv.DeployGiteaInput{
		UseExistingCluster:    flagVals.UseExistingCluster,
		BootstrapClusterProxy: setupClusterResult.BootstrapClusterProxy,
		HelmBinaryPath:        flagVals.HelmBinaryPath,
		ChartRepoName:         e2eConfig.GetVariable(e2e.GiteaRepoNameVar),
		ChartRepoURL:          e2eConfig.GetVariable(e2e.GiteaRepoURLVar),
		ChartName:             e2eConfig.GetVariable(e2e.GiteaChartNameVar),
		ChartVersion:          e2eConfig.GetVariable(e2e.GiteaChartVersionVar),
		ValuesFilePath:        "../../data/gitea/values.yaml",
		Values: map[string]string{
			"gitea.admin.username": e2eConfig.GetVariable(e2e.GiteaUserNameVar),
			"gitea.admin.password": e2eConfig.GetVariable(e2e.GiteaUserPasswordVar),
		},
		RolloutWaitInterval: e2eConfig.GetIntervals(setupClusterResult.BootstrapClusterProxy.GetName(), "wait-gitea"),
		ServiceWaitInterval: e2eConfig.GetIntervals(setupClusterResult.BootstrapClusterProxy.GetName(), "wait-getservice"),
		AuthSecretName:      e2e.AuthSecretName,
		Username:            e2eConfig.GetVariable(e2e.GiteaUserNameVar),
		Password:            e2eConfig.GetVariable(e2e.GiteaUserPasswordVar),
	})
})

var _ = AfterSuite(func() {
	testenv.CleanupTestCluster(ctx, testenv.CleanupTestClusterInput{
		SetupTestClusterResult: *setupClusterResult,
		SkipCleanup:            flagVals.SkipCleanup,
		ArtifactFolder:         flagVals.ArtifactFolder,
	})
})

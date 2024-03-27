//go:build e2e
// +build e2e

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

package e2e

import (
	"flag"
)

type FlagValues struct {
	// ConfigPath is the path to the e2e config file.
	ConfigPath string

	// UseExistingCluster instructs the test to use the current cluster instead of creating a new one (default discovery rules apply).
	UseExistingCluster bool

	// UseEKS instructs the test to create an EKS cluster instead of using kind.
	UseEKS bool

	// ArtifactFolder is the folder to store e2e test artifacts.
	ArtifactFolder string

	// SkipCleanup prevents cleanup of test resources e.g. for debug purposes.
	SkipCleanup bool

	// HelmBinaryPath is the path to the helm binary.
	HelmBinaryPath string

	// HelmExtraValuesDir is the location where extra values files will be stored.
	HelmExtraValuesDir string

	// ChartPath is the path to the operator chart.
	ChartPath string

	// IsolatedMode instructs the test to run without ngrok and exposing the cluster to the internet. This setup will only work with CAPD
	// or other providers that run in the same network as the bootstrap cluster.
	IsolatedMode bool

	// ClusterctlBinaryPath is the path to the clusterctl binary to use.
	ClusterctlBinaryPath string

	// CNI for the cluster to use. Values include calico or kindnet.
	CNI string
}

// InitFlags is used to specify the standard flags for the e2e tests.
func InitFlags(values *FlagValues) {
	flag.StringVar(&values.ConfigPath, "e2e.config", "config/operator.yaml", "path to the e2e config file")
	flag.StringVar(&values.ArtifactFolder, "e2e.artifacts-folder", "_artifacts", "folder where e2e test artifact should be stored")
	flag.BoolVar(&values.SkipCleanup, "e2e.skip-resource-cleanup", false, "if true, the resource cleanup after tests will be skipped")
	flag.BoolVar(&values.UseExistingCluster, "e2e.use-existing-cluster", false, "if true, the test uses the current cluster instead of creating a new one (default discovery rules apply)")
	flag.BoolVar(&values.UseEKS, "e2e.use-eks", true, "if true, the test uses EKS for the management cluster")
	flag.StringVar(&values.HelmBinaryPath, "e2e.helm-binary-path", "helm", "path to the helm binary")
	flag.StringVar(&values.HelmExtraValuesDir, "e2e.helm-extra-values-path", "/tmp", "path to the extra values file")
	flag.StringVar(&values.ClusterctlBinaryPath, "e2e.clusterctl-binary-path", "helm", "path to the clusterctl binary")
	flag.StringVar(&values.ChartPath, "e2e.chart-path", "", "path to the operator chart")
	flag.BoolVar(&values.IsolatedMode, "e2e.isolated-mode", false, "if true, the test will run without ngrok and exposing the cluster to the internet. This setup will only work with CAPD or other providers that run in the same network as the bootstrap cluster.")
	flag.StringVar(&values.CNI, "e2e.cni", "calico", "specify CNI solution for the cluster. Allowed values: calico, kindnet")
}

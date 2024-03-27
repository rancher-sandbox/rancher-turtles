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
	_ "embed"
)

var (
	//go:embed data/capi-operator/capi-providers.yaml
	CapiProviders []byte

	//go:embed data/capi-operator/capv-provider.yaml
	CapvProvider []byte

	//go:embed data/capi-operator/full-providers.yaml
	FullProviders []byte

	//go:embed data/capi-operator/capa-variables.yaml
	AWSProviderSecret []byte

	//go:embed data/capi-operator/capz-identity-secret.yaml
	AzureIdentitySecret []byte

	//go:embed data/capi-operator/capv-variables.yaml
	VSphereProviderSecret []byte

	//go:embed data/rancher/ingress.yaml
	IngressConfig []byte

	//go:embed data/rancher/rancher-service-patch.yaml
	RancherServicePatch []byte

	//go:embed data/rancher/ingress-class-patch.yaml
	IngressClassPatch []byte

	//go:embed data/rancher/rancher-setting-patch.yaml
	RancherSettingPatch []byte

	//go:embed data/rancher/nginx-ingress.yaml
	NginxIngress []byte

	//go:embed data/rancher/azure-rke-config.yaml
	V2ProvAzureRkeConfig []byte

	//go:embed data/rancher/azure-cluster.yaml
	V2ProvAzureCluster []byte

	//go:embed data/cluster-templates/docker-kubeadm.yaml
	CAPIDockerKubeadm []byte

	//go:embed data/cluster-templates/aws-eks-mmp.yaml
	CAPIAwsEKSMMP []byte

	//go:embed data/cluster-templates/azure-aks-mmp.yaml
	CAPIAzureAKSMMP []byte

	//go:embed data/cluster-templates/vsphere-kubeadm.yaml
	CAPIvSphereKubeadm []byte

	//go:embed data/cluster-templates/cni-calico.yaml
	CalicoCNI []byte

	//go:embed data/cluster-templates/cni-kindnet.yaml
	KindnetCNI []byte
)

const (
	RancherTurtlesNamespace = "rancher-turtles-system"
	RancherNamespace        = "cattle-system"
	NginxIngressNamespace   = "ingress-nginx"
)

const (
	KubernetesManagementVersionVar = "KUBERNETES_MANAGEMENT_VERSION"

	KubernetesVersionVar   = "KUBERNETES_VERSION"
	RancherFeaturesVar     = "RANCHER_FEATURES"
	RancherHostnameVar     = "RANCHER_HOSTNAME"
	RancherVersionVar      = "RANCHER_VERSION"
	RancherPathVar         = "RANCHER_PATH"
	RancherUrlVar          = "RANCHER_URL"
	RancherRepoNameVar     = "RANCHER_REPO_NAME"
	RancherPasswordVar     = "RANCHER_PASSWORD"
	CertManagerUrlVar      = "CERT_MANAGER_URL"
	CertManagerRepoNameVar = "CERT_MANAGER_REPO_NAME"
	CertManagerPathVar     = "CERT_MANAGER_PATH"
	CapiInfrastructureVar  = "CAPI_INFRASTRUCTURE"

	NgrokRepoNameVar  = "NGROK_REPO_NAME"
	NgrokUrlVar       = "NGROK_URL"
	NgrokPathVar      = "NGROK_PATH"
	NgrokApiKeyVar    = "NGROK_API_KEY"
	NgrokAuthTokenVar = "NGROK_AUTHTOKEN"

	GiteaRepoNameVar     = "GITEA_REPO_NAME"
	GiteaRepoURLVar      = "GITEA_REPO_URL"
	GiteaChartNameVar    = "GITEA_CHART_NAME"
	GiteaChartVersionVar = "GITEA_CHART_VERSION"
	GiteaUserNameVar     = "GITEA_USER_NAME"
	GiteaUserPasswordVar = "GITEA_USER_PWD"

	RKE2VersionVar = "RKE2_VERSION"

	CapaEncodedCredentialsVar = "CAPA_ENCODED_CREDS"
	AzureSubIDVar             = "AZURE_SUBSCRIPTION_ID"
	AzureClientIDVar          = "AZURE_CLIENT_ID"
	AzureClientSecretVar      = "AZURE_CLIENT_SECRET"

	AuthSecretName = "basic-auth-secret"

	ShortTestLabel = "short"
	FullTestLabel  = "full"
	DontRunLabel   = "dontrun"
	LocalTestLabel = "local"

	CapiClusterOwnerLabel          = "cluster-api.cattle.io/capi-cluster-owner"
	CapiClusterOwnerNamespaceLabel = "cluster-api.cattle.io/capi-cluster-owner-ns"
	OwnedLabelName                 = "cluster-api.cattle.io/owned"
)

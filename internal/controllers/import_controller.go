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

package controllers

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	errorutils "k8s.io/apimachinery/pkg/util/errors"
	yamlDecoder "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/predicates"
	utilyaml "sigs.k8s.io/cluster-api/util/yaml"

	managementv3 "github.com/rancher-sandbox/rancher-turtles/internal/rancher/management/v3"
	provisioningv1 "github.com/rancher-sandbox/rancher-turtles/internal/rancher/provisioning/v1"
	"github.com/rancher-sandbox/rancher-turtles/util"
	turtlesannotations "github.com/rancher-sandbox/rancher-turtles/util/annotations"
	turtlesnaming "github.com/rancher-sandbox/rancher-turtles/util/naming"
	turtlespredicates "github.com/rancher-sandbox/rancher-turtles/util/predicates"
)

const (
	importLabelName = "cluster-api.cattle.io/rancher-auto-import"
	ownedLabelName  = "cluster-api.cattle.io/owned"

	defaultRequeueDuration = 1 * time.Minute
)

// CAPIImportReconciler represents a reconciler for importing CAPI clusters in Rancher.
type CAPIImportReconciler struct {
	Client             client.Client
	RancherClient      client.Client
	recorder           record.EventRecorder
	WatchFilterValue   string
	Scheme             *runtime.Scheme
	InsecureSkipVerify bool

	controller         controller.Controller
	externalTracker    external.ObjectTracker
	remoteClientGetter remote.ClusterClientGetter
}

// SetupWithManager sets up reconciler with manager.
func (r *CAPIImportReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	log := log.FromContext(ctx)

	if r.remoteClientGetter == nil {
		r.remoteClientGetter = remote.NewClusterClient
	}

	capiPredicates := predicates.All(log,
		predicates.ResourceHasFilterLabel(log, r.WatchFilterValue),
		turtlespredicates.ClusterWithoutImportedAnnotation(log),
		turtlespredicates.ClusterWithReadyControlPlane(log),
		turtlespredicates.ClusterOrNamespaceWithImportLabel(ctx, log, r.Client, importLabelName),
	)

	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.Cluster{}).
		WithOptions(options).
		WithEventFilter(capiPredicates).
		Build(r)
	if err != nil {
		return fmt.Errorf("creating new controller: %w", err)
	}

	// Watch Rancher provisioningv2 clusters
	// NOTE: we will import the types from rancher in the future
	err = c.Watch(
		source.Kind(mgr.GetCache(), &provisioningv1.Cluster{}),
		handler.EnqueueRequestsFromMapFunc(r.rancherClusterToCapiCluster(ctx, capiPredicates)),
		//&handler.EnqueueRequestForOwner{OwnerType: &clusterv1.Cluster{}},
	)
	if err != nil {
		return fmt.Errorf("adding watch for Rancher cluster: %w", err)
	}

	ns := &corev1.Namespace{}
	err = c.Watch(
		source.Kind(mgr.GetCache(), ns),
		handler.EnqueueRequestsFromMapFunc(r.namespaceToCapiClusters(ctx, capiPredicates)),
	)

	if err != nil {
		return fmt.Errorf("adding watch for namespaces: %w", err)
	}

	r.recorder = mgr.GetEventRecorderFor("rancher-turtles")
	r.controller = c
	r.externalTracker = external.ObjectTracker{
		Controller: c,
	}

	return nil
}

// +kubebuilder:rbac:groups="",resources=secrets;events;configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;create;update
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch;create;update;delete;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinepools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinepools/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinepools/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch;delete;create;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinesets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinesets/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinesets/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=provisioning.cattle.io,resources=clusters;clusters/status,verbs=get;list;watch;create;update;delete;patch
// +kubebuilder:rbac:groups=management.cattle.io,resources=clusterregistrationtokens;clusterregistrationtokens/status,verbs=get;list;watch

// Reconcile reconciles a CAPI cluster, creating a Rancher cluster if needed and applying the import manifests.
func (r *CAPIImportReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, reterr error) {
	log := log.FromContext(ctx)
	log.Info("Reconciling CAPI cluster")

	capiCluster := &clusterv1.Cluster{}
	if err := r.Client.Get(ctx, req.NamespacedName, capiCluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{Requeue: true}, nil
		}

		return ctrl.Result{Requeue: true}, err
	}

	patchBase := client.MergeFromWithOptions(capiCluster.DeepCopy(), client.MergeFromWithOptimisticLock{})

	log = log.WithValues("cluster", capiCluster.Name)

	// Wait for controlplane to be ready. This should never be false as the predicates
	// do the filtering.
	if !capiCluster.Status.ControlPlaneReady && !conditions.IsTrue(capiCluster, clusterv1.ControlPlaneReadyCondition) {
		log.Info("clusters control plane is not ready, requeue")
		return ctrl.Result{RequeueAfter: defaultRequeueDuration}, nil
	}

	// Collect errors as an aggregate to return together after all patches have been performed.
	var errs []error

	result, err := r.reconcile(ctx, capiCluster)
	if err != nil {
		errs = append(errs, fmt.Errorf("error reconciling cluster: %w", err))
	}

	if err := r.Client.Patch(ctx, capiCluster, patchBase); err != nil {
		errs = append(errs, fmt.Errorf("failed to patch cluster: %w", err))
	}

	if len(errs) > 0 {
		return ctrl.Result{}, errorutils.NewAggregate(errs)
	}

	return result, nil
}

func (r *CAPIImportReconciler) reconcile(ctx context.Context, capiCluster *clusterv1.Cluster) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// fetch the rancher cluster
	rancherCluster := &provisioningv1.Cluster{ObjectMeta: metav1.ObjectMeta{
		Namespace: capiCluster.Namespace,
		Name:      turtlesnaming.Name(capiCluster.Name).ToRancherName(),
	}}

	err := r.RancherClient.Get(ctx, client.ObjectKeyFromObject(rancherCluster), rancherCluster)
	if client.IgnoreNotFound(err) != nil {
		log.Error(err, fmt.Sprintf("Unable to fetch rancher cluster %s", client.ObjectKeyFromObject(rancherCluster)))
		return ctrl.Result{Requeue: true}, err
	}

	if !rancherCluster.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, capiCluster)
	}

	return r.reconcileNormal(ctx, capiCluster, rancherCluster)
}

func (r *CAPIImportReconciler) reconcileNormal(ctx context.Context, capiCluster *clusterv1.Cluster,
	rancherCluster *provisioningv1.Cluster,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	err := r.RancherClient.Get(ctx, client.ObjectKeyFromObject(rancherCluster), rancherCluster)
	if apierrors.IsNotFound(err) {
		shouldImport, err := util.ShouldAutoImport(ctx, log, r.Client, capiCluster, importLabelName)
		if err != nil {
			return ctrl.Result{}, err
		}

		if !shouldImport {
			log.Info("not auto importing cluster as namespace or cluster isn't marked auto import")
			return ctrl.Result{}, nil
		}

		if err := r.RancherClient.Create(ctx, &provisioningv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      turtlesnaming.Name(capiCluster.Name).ToRancherName(),
				Namespace: capiCluster.Namespace,
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       clusterv1.ClusterKind,
					Name:       capiCluster.Name,
					UID:        capiCluster.UID,
				}},
				Labels: map[string]string{
					ownedLabelName: "",
				},
			},
		}); err != nil {
			return ctrl.Result{}, fmt.Errorf("error creating rancher cluster: %w", err)
		}

		return ctrl.Result{Requeue: true}, nil
	}

	if err != nil {
		log.Error(err, fmt.Sprintf("Unable to fetch rancher cluster %s", client.ObjectKeyFromObject(rancherCluster)))

		return ctrl.Result{}, err
	}

	if rancherCluster.Status.ClusterName == "" {
		log.Info("cluster name not set yet, requeue")
		return ctrl.Result{Requeue: true}, nil
	}

	log.Info("found cluster name", "name", rancherCluster.Status.ClusterName)

	if rancherCluster.Status.AgentDeployed {
		log.Info("agent already deployed, no action needed")
		return ctrl.Result{}, nil
	}

	// get the registration manifest
	manifest, err := r.getClusterRegistrationManifest(ctx, rancherCluster.Status.ClusterName, capiCluster.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	if manifest == "" {
		log.Info("Import manifest URL not set yet, requeue")
		return ctrl.Result{Requeue: true}, nil
	}

	log.Info("Creating import manifest")

	remoteClient, err := r.remoteClientGetter(ctx, capiCluster.Name, r.Client, client.ObjectKeyFromObject(capiCluster))
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting remote cluster client: %w", err)
	}

	if err := r.createImportManifest(ctx, remoteClient, strings.NewReader(manifest)); err != nil {
		return ctrl.Result{}, fmt.Errorf("creating import manifest: %w", err)
	}

	log.Info("Successfully applied import manifest")

	return ctrl.Result{}, nil
}

func (r *CAPIImportReconciler) reconcileDelete(ctx context.Context, capiCluster *clusterv1.Cluster) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Reconciling rancher cluster deletion")

	// If the Rancher Cluster was already imported, then annotate the CAPI cluster so that we don't auto-import again.
	log.Info(fmt.Sprintf("Rancher cluster is being removed, annotating CAPI cluster %s with %s",
		capiCluster.Name,
		turtlesannotations.ClusterImportedAnnotation))

	annotations := capiCluster.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}

	annotations[turtlesannotations.ClusterImportedAnnotation] = "true"
	capiCluster.SetAnnotations(annotations)

	return ctrl.Result{}, nil
}

func (r *CAPIImportReconciler) getClusterRegistrationManifest(ctx context.Context, clusterName, namespace string) (string, error) {
	log := log.FromContext(ctx)

	token := &managementv3.ClusterRegistrationToken{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: namespace,
		},
		Spec: managementv3.ClusterRegistrationTokenSpec{
			ClusterName: clusterName,
		},
	}
	err := r.RancherClient.Get(ctx, client.ObjectKeyFromObject(token), token)

	if client.IgnoreNotFound(err) != nil {
		return "", fmt.Errorf("error getting registration token for cluster %s: %w", clusterName, err)
	} else if err != nil {
		if err := r.RancherClient.Create(ctx, token); err != nil {
			return "", fmt.Errorf("failed to create cluster registration token for cluster %s: %w", clusterName, err)
		}
	}

	if token.Status.ManifestURL == "" {
		return "", nil
	}

	manifestData, err := r.downloadManifest(token.Status.ManifestURL)
	if err != nil {
		log.Error(err, "failed downloading import manifest")
		return "", err
	}

	return manifestData, nil
}

func (r *CAPIImportReconciler) rancherClusterToCapiCluster(ctx context.Context, clusterPredicate predicate.Funcs) handler.MapFunc {
	log := log.FromContext(ctx)

	return func(_ context.Context, o client.Object) []ctrl.Request {
		capiCluster := &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{
			Name:      turtlesnaming.Name(o.GetName()).ToCapiName(),
			Namespace: o.GetNamespace(),
		}}
		if err := r.Client.Get(ctx, client.ObjectKeyFromObject(capiCluster), capiCluster); err != nil {
			if !apierrors.IsNotFound(err) {
				log.Error(err, "getting capi cluster")
			}

			return nil
		}

		if !clusterPredicate.Generic(event.GenericEvent{Object: capiCluster}) {
			return nil
		}

		return []ctrl.Request{{NamespacedName: client.ObjectKey{Namespace: capiCluster.Namespace, Name: capiCluster.Name}}}
	}
}

func (r *CAPIImportReconciler) namespaceToCapiClusters(ctx context.Context, clusterPredicate predicate.Funcs) handler.MapFunc {
	log := log.FromContext(ctx)

	return func(_ context.Context, o client.Object) []ctrl.Request {
		ns, ok := o.(*corev1.Namespace)
		if !ok {
			log.Error(nil, fmt.Sprintf("Expected a Namespace but got a %T", o))
			return nil
		}

		_, autoImport := util.ShouldImport(ns, importLabelName)
		if !autoImport {
			log.V(2).Info("Namespace doesn't have import annotation label with a true value, skipping")
			return nil
		}

		capiClusters := &clusterv1.ClusterList{}
		if err := r.Client.List(ctx, capiClusters, client.InNamespace(o.GetNamespace())); err != nil {
			log.Error(err, "getting capi cluster")
			return nil
		}

		if len(capiClusters.Items) == 0 {
			log.V(2).Info("No CAPI clusters in namespace, no action")
			return nil
		}

		reqs := []ctrl.Request{}

		for _, cluster := range capiClusters.Items {
			cluster := cluster
			if !clusterPredicate.Generic(event.GenericEvent{Object: &cluster}) {
				continue
			}

			reqs = append(reqs, ctrl.Request{
				NamespacedName: client.ObjectKey{
					Namespace: cluster.Namespace,
					Name:      cluster.Name,
				},
			})
		}

		return reqs
	}
}

func (r *CAPIImportReconciler) downloadManifest(url string) (string, error) {
	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: r.InsecureSkipVerify, //nolint:gosec
		},
	}}

	resp, err := client.Get(url) //nolint:gosec,noctx
	if err != nil {
		return "", fmt.Errorf("downloading manifest: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading manifest: %w", err)
	}

	return string(data), err
}

func (r *CAPIImportReconciler) createImportManifest(ctx context.Context, remoteClient client.Client, in io.Reader) error {
	reader := yamlDecoder.NewYAMLReader(bufio.NewReaderSize(in, 4096))

	for {
		raw, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return err
		}

		if err := r.createRawManifest(ctx, remoteClient, raw); err != nil {
			return err
		}
	}

	return nil
}

func (r *CAPIImportReconciler) createRawManifest(ctx context.Context, remoteClient client.Client, bytes []byte) error {
	items, err := utilyaml.ToUnstructured(bytes)
	if err != nil {
		return fmt.Errorf("error unmarshalling bytes or empty object passed: %w", err)
	}

	for _, obj := range items {
		if err := r.createObject(ctx, remoteClient, obj.DeepCopy()); err != nil {
			return err
		}
	}

	return nil
}

func (r *CAPIImportReconciler) createObject(ctx context.Context, c client.Client, obj client.Object) error {
	log := log.FromContext(ctx)
	gvk := obj.GetObjectKind().GroupVersionKind()

	err := c.Create(ctx, obj)
	if apierrors.IsAlreadyExists(err) {
		log.V(4).Info("object already exists in remote cluster", "gvk", gvk, "name", obj.GetName(), "namespace", obj.GetNamespace())
		return nil
	}

	if err != nil {
		return fmt.Errorf("creating object in remote cluster: %w", err)
	}

	log.V(4).Info("object was created", "gvk", gvk, "name", obj.GetName(), "namespace", obj.GetNamespace())

	return nil
}

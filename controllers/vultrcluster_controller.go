/*

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
	"context"
	"fmt"
	"os"

	vultr "github.com/JamesClonk/vultr/lib"
	"github.com/go-logr/logr"
	"github.com/labstack/gommon/log"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1alpha2 "github.com/yukirii/cluster-api-provider-vultr/api/v1alpha2"
)

// VultrClusterReconciler reconciles a VultrCluster object
type VultrClusterReconciler struct {
	client.Client
	Log      logr.Logger
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vultrclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vultrclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch

func (r *VultrClusterReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx := context.Background()
	log := r.Log.WithValues("vultrcluster", req.NamespacedName)

	// Fetch the VultrCluster.
	vultrCluster := &infrav1alpha2.VultrCluster{}
	err := r.Get(ctx, req.NamespacedName, vultrCluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Fetch the Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, vultrCluster.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Info("Cluster Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}

	log = r.Log.WithValues("cluster", cluster.Name)

	patchHelper, err := patch.NewHelper(vultrCluster, r)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer func() {
		err := patchHelper.Patch(ctx, vultrCluster)
		if err != nil && reterr == nil {
			reterr = err
		}
	}()

	if !vultrCluster.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileClusterDelete(log, cluster, vultrCluster)
	}

	return r.reconcileCluster(log, cluster, vultrCluster)
}

func (r *VultrClusterReconciler) reconcileClusterDelete(logger logr.Logger, cluster *clusterv1.Cluster, vultrCluster *infrav1alpha2.VultrCluster) (ctrl.Result, error) {
	log.Info("Reconciling Cluster Delete")

	apiKey := os.Getenv("VULTR_API_KEY")
	vultrClient := vultr.NewClient(apiKey, nil)

	for _, e := range vultrCluster.Status.APIEndpoints {
		fmt.Println(e.ID)
		err := vultrClient.DestroyReservedIP(e.ID)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	vultrCluster.Finalizers = util.Filter(vultrCluster.Finalizers, infrav1alpha2.ClusterFinalizer)

	return ctrl.Result{}, nil
}

func (r *VultrClusterReconciler) reconcileCluster(logger logr.Logger, cluster *clusterv1.Cluster, vultrCluster *infrav1alpha2.VultrCluster) (ctrl.Result, error) {
	log.Info("Reconciling Cluster")

	// Add finalizer
	if !util.Contains(vultrCluster.Finalizers, infrav1alpha2.ClusterFinalizer) {
		vultrCluster.Finalizers = append(vultrCluster.Finalizers, infrav1alpha2.ClusterFinalizer)
	}

	if len(vultrCluster.Status.APIEndpoints) == 0 {
		apiKey := os.Getenv("VULTR_API_KEY")
		vultrClient := vultr.NewClient(apiKey, nil)

		id, err := vultrClient.CreateReservedIP(vultrCluster.Spec.Region, "v4", vultrCluster.Name)
		if err != nil {
			return ctrl.Result{}, err
		}

		ip, err := r.findReservedIP(vultrClient, id)
		if err != nil {
			return ctrl.Result{}, err
		}

		vultrCluster.Status.APIEndpoints = []infrav1alpha2.APIEndpoint{
			{
				ID:   id,
				Host: ip,
				Port: 6443,
			},
		}
	}

	vultrCluster.Status.Ready = true

	log.Info("Reconciled Cluster successfully")

	return ctrl.Result{}, nil
}

func (r *VultrClusterReconciler) findReservedIP(vultrClient *vultr.Client, id string) (string, error) {
	ips, err := vultrClient.ListReservedIP()
	if err != nil {
		return "", err
	}

	for _, ip := range ips {
		if ip.ID == id {
			return ip.Subnet, nil
		}
	}

	return "", nil
}

func (r *VultrClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1alpha2.VultrCluster{}).
		Complete(r)
}

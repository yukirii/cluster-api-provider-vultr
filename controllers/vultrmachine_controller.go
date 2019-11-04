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
	"encoding/base64"
	"fmt"
	"os"

	vultr "github.com/JamesClonk/vultr/lib"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/cluster-api/controllers/noderefutil"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrastructurev1alpha2 "github.com/yukirii/cluster-api-provider-vultr/api/v1alpha2"
	infrav1alpha2 "github.com/yukirii/cluster-api-provider-vultr/api/v1alpha2"
)

// VultrMachineReconciler reconciles a VultrMachine object
type VultrMachineReconciler struct {
	client.Client
	Log logr.Logger
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vultrmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vultrmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch

func (r *VultrMachineReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx := context.Background()
	log := r.Log.WithValues("vultrmachine", req.NamespacedName)

	// Fetch the VultrMachine.
	vultrMachine := &infrav1alpha2.VultrMachine{}
	err := r.Get(ctx, req.NamespacedName, vultrMachine)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Fetch the Machine.
	machine, err := util.GetOwnerMachine(ctx, r.Client, vultrMachine.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if machine == nil {
		log.Info("Machine Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}

	log = r.Log.WithValues("machine", machine.Name)

	// Fetch the Cluster.
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
	if err != nil {
		log.Info("Machine is missing cluster label or cluster does not exist")
		return ctrl.Result{}, nil
	}

	log = r.Log.WithValues("cluster", cluster.Name)

	// Fetch the VultrCluster.
	vultrCluster := &infrav1alpha2.VultrCluster{}
	vultrClusterName := client.ObjectKey{
		Namespace: vultrMachine.Namespace,
		Name:      cluster.Spec.InfrastructureRef.Name,
	}
	if err := r.Client.Get(ctx, vultrClusterName, vultrCluster); err != nil {
		log.Info("VultrCluster is not available yet.")
		return ctrl.Result{}, nil
	}

	patchHelper, err := patch.NewHelper(vultrMachine, r)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer func() {
		err := patchHelper.Patch(ctx, vultrMachine)
		if err != nil && reterr == nil {
			reterr = err
		}
	}()

	if !vultrMachine.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(log, cluster, vultrCluster, machine, vultrMachine)
	}

	return r.reconcileNormal(log, cluster, vultrCluster, machine, vultrMachine)
}

func (r *VultrMachineReconciler) reconcileDelete(log logr.Logger,
	cluster *clusterv1.Cluster, vultrCluster *infrav1alpha2.VultrCluster,
	machine *clusterv1.Machine, vultrMachine *infrav1alpha2.VultrMachine) (ctrl.Result, error) {
	log.Info("Reconciling Machine Delete")

	apiKey := os.Getenv("VULTR_API_KEY")
	vultrClient := vultr.NewClient(apiKey, nil)

	server, err := r.findServer(vultrClient, vultrCluster, vultrMachine)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = vultrClient.DeleteServer(server.ID)
	if err != nil {
		return ctrl.Result{}, err
	}

	vultrMachine.Finalizers = util.Filter(vultrMachine.Finalizers, infrav1alpha2.MachineFinalizer)

	return ctrl.Result{}, nil
}

func (r *VultrMachineReconciler) reconcileNormal(log logr.Logger,
	cluster *clusterv1.Cluster, vultrCluster *infrav1alpha2.VultrCluster,
	machine *clusterv1.Machine, vultrMachine *infrav1alpha2.VultrMachine) (ctrl.Result, error) {
	log.Info("Reconciling Machine")

	// Add finalizer
	if !util.Contains(vultrMachine.Finalizers, infrav1alpha2.MachineFinalizer) {
		vultrMachine.Finalizers = append(vultrMachine.Finalizers, infrav1alpha2.MachineFinalizer)
	}

	if cluster.Status.InfrastructureReady != true {
		log.Info("Cluster infrastructure is not ready yet.")
		return ctrl.Result{}, nil
	}

	if machine.Spec.Bootstrap.Data == nil {
		log.Info("Bootstrap data is not yet available.")
		return ctrl.Result{}, nil
	}

	server, err := r.getOrCreate(vultrCluster, machine, vultrMachine)
	if err != nil {
		return ctrl.Result{}, err
	}

	vultrMachine.Spec.ProviderID = pointer.StringPtr(fmt.Sprintf("vultr:////%s", server.ID))
	vultrMachine.Status.Ready = true

	return ctrl.Result{}, nil
}

func (r *VultrMachineReconciler) findServer(vultrClient *vultr.Client, vultrCluster *infrav1alpha2.VultrCluster, vultrMachine *infrav1alpha2.VultrMachine) (*vultr.Server, error) {
	providerID := ""
	if vultrMachine.Spec.ProviderID != nil {
		providerID = *vultrMachine.Spec.ProviderID
	}

	// Parse the ProviderID.
	pid, err := noderefutil.NewProviderID(providerID)
	if err != nil && err != noderefutil.ErrEmptyProviderID {
		return nil, errors.Wrapf(err, "failed to parse Spec.ProviderID")
	}

	// If the ProviderID populated, get the server using the ID.
	if err == nil {
		server, err := vultrClient.GetServer(pid.ID())
		if err != nil && err.Error() == "Invalid server." {
			return nil, nil
		}

		if err == nil {
			return &server, nil
		}
	}

	// If the ProviderID is empty, try to get the server using tag and name (label).
	tag := fmt.Sprintf("%s:owned", vultrCluster.Name)
	servers, err := vultrClient.GetServersByTag(tag)
	if err != nil {
		return nil, err
	}

	for _, s := range servers {
		if s.Name == vultrMachine.GetName() {
			return &s, nil
		}
	}

	return nil, nil
}

func (r *VultrMachineReconciler) getOrCreate(vultrCluster *infrav1alpha2.VultrCluster,
	machine *clusterv1.Machine, vultrMachine *infrav1alpha2.VultrMachine) (*vultr.Server, error) {
	apiKey := os.Getenv("VULTR_API_KEY")
	vultrClient := vultr.NewClient(apiKey, nil)

	server, err := r.findServer(vultrClient, vultrCluster, vultrMachine)
	if err != nil {
		return nil, err
	}

	// Create a new server if we couldn't get a server
	if server == nil {
		sshKeyID, err := r.getSSHKeyIDByName(vultrClient, &vultrMachine.Spec.SSHKeyName)
		if err != nil {
			return nil, err
		}

		userdata, err := base64.StdEncoding.DecodeString(*machine.Spec.Bootstrap.Data)
		if err != nil {
			return nil, err
		}

		options := &vultr.ServerOptions{
			ReservedIP: vultrCluster.Status.APIEndpoints[0].Host,
			UserData:   string(userdata),
			SSHKey:     sshKeyID,
			Tag:        fmt.Sprintf("%s:owned", vultrCluster.Name),
		}

		if vultrMachine.Spec.ScriptID != 0 {
			options.Script = vultrMachine.Spec.ScriptID
		}

		srv, err := vultrClient.CreateServer(machine.Name,
			vultrCluster.Spec.Region, vultrMachine.Spec.PlanID, vultrMachine.Spec.OSID,
			options)

		server = &srv
	}

	return server, nil
}

func (r *VultrMachineReconciler) getSSHKeyIDByName(vultrClient *vultr.Client, keyName *string) (string, error) {
	keys, err := vultrClient.GetSSHKeys()
	if err != nil {
		return "", err
	}

	for _, k := range keys {
		if k.Name == *keyName {
			return k.ID, nil
		}
	}
	return "", fmt.Errorf("SSH Key '%s' is not found.", *keyName)
}

func (r *VultrMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha2.VultrMachine{}).
		Complete(r)
}

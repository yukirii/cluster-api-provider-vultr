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

	vultr "github.com/JamesClonk/vultr/lib"
	"github.com/go-logr/logr"
	"github.com/labstack/gommon/log"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/cluster-api/controllers/noderefutil"
	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrastructurev1alpha2 "github.com/yukirii/cluster-api-provider-vultr/api/v1alpha2"
	infrav1alpha2 "github.com/yukirii/cluster-api-provider-vultr/api/v1alpha2"
	"github.com/yukirii/cluster-api-provider-vultr/pkg/cloud/scope"
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

	log = r.Log.WithValues("vultrCluster", vultrCluster.Name)

	// Create the machine scope
	machineScope, err := scope.NewMachineScope(scope.MachineScopeParams{
		Client:       r.Client,
		Logger:       log,
		Cluster:      cluster,
		Machine:      machine,
		VultrCluster: vultrCluster,
		VultrMachine: vultrMachine,
	})
	if err != nil {
		return ctrl.Result{}, errors.Errorf("failed to create scope: %v", err)
	}

	defer func() {
		err := machineScope.Close()
		if err != nil && reterr == nil {
			reterr = err
		}
	}()

	if !vultrMachine.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(machineScope)
	}

	return r.reconcileNormal(machineScope)
}

func (r *VultrMachineReconciler) reconcileDelete(machineScope *scope.MachineScope) (ctrl.Result, error) {
	log.Info("Reconciling Machine Delete")

	server, err := r.findServer(machineScope)
	if err != nil {
		return ctrl.Result{}, err
	}

	if server != nil {
		err = machineScope.VultrClient.DeleteServer(server.ID)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	machineScope.VultrMachine.Finalizers = util.Filter(machineScope.VultrMachine.Finalizers, infrav1alpha2.MachineFinalizer)

	return ctrl.Result{}, nil
}

func (r *VultrMachineReconciler) reconcileNormal(machineScope *scope.MachineScope) (ctrl.Result, error) {
	log.Info("Reconciling Machine")

	// Add finalizer
	if !util.Contains(machineScope.VultrMachine.Finalizers, infrav1alpha2.MachineFinalizer) {
		machineScope.VultrMachine.Finalizers = append(machineScope.VultrMachine.Finalizers, infrav1alpha2.MachineFinalizer)
	}

	if machineScope.Cluster.Status.InfrastructureReady != true {
		log.Info("Cluster infrastructure is not ready yet.")
		return ctrl.Result{}, nil
	}

	if machineScope.Machine.Spec.Bootstrap.Data == nil {
		log.Info("Bootstrap data is not yet available.")
		return ctrl.Result{}, nil
	}

	server, err := r.getOrCreate(machineScope)
	if err != nil {
		return ctrl.Result{}, err
	}

	machineScope.VultrMachine.Spec.ProviderID = pointer.StringPtr(fmt.Sprintf("vultr:////%s", server.ID))
	machineScope.VultrMachine.Status.Ready = true

	return ctrl.Result{}, nil
}

func (r *VultrMachineReconciler) findServer(machineScope *scope.MachineScope) (*vultr.Server, error) {
	providerID := ""
	if machineScope.VultrMachine.Spec.ProviderID != nil {
		providerID = *machineScope.VultrMachine.Spec.ProviderID
	}

	// Parse the ProviderID.
	pid, err := noderefutil.NewProviderID(providerID)
	if err != nil && err != noderefutil.ErrEmptyProviderID {
		return nil, errors.Wrapf(err, "failed to parse Spec.ProviderID")
	}

	// If the ProviderID populated, get the server using the ID.
	if err == nil {
		server, err := machineScope.VultrClient.GetServer(pid.ID())
		if err != nil && err.Error() == "Invalid server." {
			return nil, nil
		}

		if err == nil {
			return &server, nil
		}
	}

	// If the ProviderID is empty, try to get the server using tag and name (label).
	tag := fmt.Sprintf("%s:owned", machineScope.VultrCluster.Name)
	servers, err := machineScope.VultrClient.GetServersByTag(tag)
	if err != nil {
		return nil, err
	}

	for _, s := range servers {
		if s.Name == machineScope.VultrMachine.GetName() {
			return &s, nil
		}
	}

	return nil, nil
}

func (r *VultrMachineReconciler) getOrCreate(machineScope *scope.MachineScope) (*vultr.Server, error) {
	server, err := r.findServer(machineScope)
	if err != nil {
		return nil, err
	}

	// Create a new server if we couldn't get a server
	if server == nil {
		sshKeyID, err := r.getSSHKeyIDByName(machineScope.VultrClient, &machineScope.VultrMachine.Spec.SSHKeyName)
		if err != nil {
			return nil, err
		}

		userdata, err := base64.StdEncoding.DecodeString(*machineScope.Machine.Spec.Bootstrap.Data)
		if err != nil {
			return nil, err
		}

		options := &vultr.ServerOptions{
			Hostname: machineScope.Machine.Name,
			UserData: string(userdata),
			SSHKey:   sshKeyID,
			Tag:      fmt.Sprintf("%s:owned", machineScope.VultrCluster.Name),
		}

		// Set ReservedIP if the Machine has control-plane label
		labels := machineScope.Machine.GetLabels()
		if labels["cluster.x-k8s.io/control-plane"] == "true" {
			options.ReservedIP = machineScope.VultrCluster.Status.APIEndpoints[0].Host
		}

		// Set ScriptID if the Machine has Vultr Script ID
		if machineScope.VultrMachine.Spec.ScriptID != 0 {
			options.Script = machineScope.VultrMachine.Spec.ScriptID
		}

		srv, err := machineScope.VultrClient.CreateServer(machineScope.Machine.Name,
			machineScope.VultrCluster.Spec.Region, machineScope.VultrMachine.Spec.PlanID,
			machineScope.VultrMachine.Spec.OSID, options)
		if err != nil {
			return nil, err
		}

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

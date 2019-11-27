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

package scope

import (
	"context"
	"os"

	vultr "github.com/JamesClonk/vultr/lib"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha2"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1alpha2 "github.com/yukirii/cluster-api-provider-vultr/api/v1alpha2"
)

type MachineScopeParams struct {
	VultrClient  *vultr.Client
	Client       client.Client
	Logger       logr.Logger
	Machine      *clusterv1.Machine
	Cluster      *clusterv1.Cluster
	VultrMachine *infrav1alpha2.VultrMachine
	VultrCluster *infrav1alpha2.VultrCluster
}

type MachineScope struct {
	VultrClient  *vultr.Client
	client       client.Client
	Logger       logr.Logger
	Machine      *clusterv1.Machine
	Cluster      *clusterv1.Cluster
	VultrMachine *infrav1alpha2.VultrMachine
	VultrCluster *infrav1alpha2.VultrCluster
	patchHelper  *patch.Helper
}

// NewMachineScope creates a new Scope from the supplied parameters.
func NewMachineScope(params MachineScopeParams) (*MachineScope, error) {
	if params.Client == nil {
		return nil, errors.New("client is required when creating a MachineScope")
	}
	if params.Cluster == nil {
		return nil, errors.New("cluster is required when creating a MachineScope")
	}
	if params.Machine == nil {
		return nil, errors.New("machine is required when creating a MachineScope")
	}
	if params.VultrCluster == nil {
		return nil, errors.New("vultr cluster is required when creating a MachineScope")
	}
	if params.VultrMachine == nil {
		return nil, errors.New("vultr machine is required when creating a MachineScope")
	}

	apiKey := os.Getenv("VULTR_API_KEY")
	params.VultrClient = vultr.NewClient(apiKey, nil)

	helper, err := patch.NewHelper(params.VultrMachine, params.Client)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init patch helper")
	}

	return &MachineScope{
		client:       params.Client,
		Logger:       params.Logger,
		Cluster:      params.Cluster,
		Machine:      params.Machine,
		VultrCluster: params.VultrCluster,
		VultrMachine: params.VultrMachine,
		VultrClient:  params.VultrClient,
		patchHelper:  helper,
	}, nil
}

func (s *MachineScope) Close() error {
	return s.patchHelper.Patch(context.TODO(), s.VultrMachine)
}

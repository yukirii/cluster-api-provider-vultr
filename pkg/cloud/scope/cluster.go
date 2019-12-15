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

	"github.com/pkg/errors"
	infrav1alpha2 "github.com/yukirii/cluster-api-provider-vultr/api/v1alpha2"

	vultr "github.com/JamesClonk/vultr/lib"
	"github.com/go-logr/logr"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ClusterScopeParams struct {
	VultrClient  *vultr.Client
	Client       client.Client
	Logger       logr.Logger
	VultrCluster *infrav1alpha2.VultrCluster
}

type ClusterScope struct {
	VultrClient  *vultr.Client
	client       client.Client
	Logger       logr.Logger
	VultrCluster *infrav1alpha2.VultrCluster
	patchHelper  *patch.Helper
}

// NewClusterScope creates a new Scope from the supplied parameters.
func NewClusterScope(params ClusterScopeParams) (*ClusterScope, error) {
	apiKey := os.Getenv("VULTR_API_KEY")
	params.VultrClient = vultr.NewClient(apiKey, nil)

	helper, err := patch.NewHelper(params.VultrCluster, params.Client)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init patch helper")
	}

	return &ClusterScope{
		client:       params.Client,
		Logger:       params.Logger,
		VultrCluster: params.VultrCluster,
		VultrClient:  params.VultrClient,
		patchHelper:  helper,
	}, nil
}

func (s *ClusterScope) Close() error {
	return s.patchHelper.Patch(context.TODO(), s.VultrCluster)
}

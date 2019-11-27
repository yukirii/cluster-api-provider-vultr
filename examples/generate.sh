#!/bin/bash

# Versions
CAPV_VERSION="v0.0.0"
CAPI_VERSION="v0.2.7"
CABPK_VERSION="v0.1.5"
CALICO_VERSION="v3.8"

# Vultr Settings
export SSH_KEY_NAME="${SSH_KEY_NAME:-default}"
export VULTR_REGION="${VULTR_REGION:-25}"   # Tokyo
export VULTR_B64ENCODED_API_KEY=$(echo ${VULTR_API_KEY} | base64)

# Cluster Settings
export KUBERNETES_VERSION="${KUBERNETES_VERSION:-v1.16.2}"
export CLUSTER_NAME="${CLUSTER_NAME:-capi}"

# Machine Settings
# VPSPLANID 203: 2 vCPU, 4096MB RAM, 80GB SSD, 3.00 TB BW
export CONTROL_PLANE_PLAN_ID="${CONTROL_PLANE_PLAN_ID:-203}"
export WORKER_PLAN_ID="${WORKER_PLAN_ID:-203}"
# OSID 338: Ubuntu 19.04 x64
export CONTROL_PLANE_OS_ID="${CONTROL_PLANE_OS_ID:-338}"
export WORKER_OS_ID="${WORKER_OS_ID:-338}"

# Output Settings
SOURCE_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null && pwd )"
OUTPUT_DIR=${OUTPUT_DIR:-${SOURCE_DIR}/_out}

COMPONENTS_CLUSTER_API_GENERATED_FILE=${SOURCE_DIR}/provider-components/provider-components-cluster-api.yaml
COMPONENTS_KUBEADM_GENERATED_FILE=${SOURCE_DIR}/provider-components/provider-components-kubeadm.yaml
COMPONENTS_VULTR_GENERATED_FILE=${SOURCE_DIR}/provider-components/provider-components-vultr.yaml

CLUSTER_GENERATED_FILE=${OUTPUT_DIR}/cluster.yaml
CONTROLPLANE_GENERATED_FILE=${OUTPUT_DIR}/controlplane.yaml
MACHINES_GENERATED_FILE=${OUTPUT_DIR}/machines.yaml
PROVIDER_COMPONENTS_GENERATED_FILE=${OUTPUT_DIR}/provider-components.yaml
ADDON_GENERATED_FILE=${OUTPUT_DIR}/addon.yaml

if [ -d "$OUTPUT_DIR" ]; then
  echo "ERR: Folder ${OUTPUT_DIR} already exists. Delete it manually before running this script."
  exit 1
fi

mkdir -p "${OUTPUT_DIR}"

# Generate cluster manifest
kustomize build "${SOURCE_DIR}/cluster" | envsubst > "${CLUSTER_GENERATED_FILE}"
echo "Generated ${CLUSTER_GENERATED_FILE}"

# Generate controlplane manifest
kustomize build "${SOURCE_DIR}/controlplane" | envsubst > "${CONTROLPLANE_GENERATED_FILE}"
echo "Generated ${CONTROLPLANE_GENERATED_FILE}"

# Generate machine manifest
kustomize build "${SOURCE_DIR}/machine" | envsubst > "${MACHINES_GENERATED_FILE}"
echo "Generated ${MACHINES_GENERATED_FILE}"

# Download & Generate provider-components.yaml
# Cluster API Provider Vultr
kustomize build "${SOURCE_DIR}/../config/default" | envsubst > "${COMPONENTS_VULTR_GENERATED_FILE}"
echo "Generated ${COMPONENTS_VULTR_GENERATED_FILE}"

## Cluster API
kustomize build "github.com/kubernetes-sigs/cluster-api//config/default/?ref=${CAPI_VERSION}" > "${COMPONENTS_CLUSTER_API_GENERATED_FILE}"
echo "Generated ${COMPONENTS_CLUSTER_API_GENERATED_FILE}"

## Cluster API Bootstrap Provider kubeadm
kustomize build "github.com/kubernetes-sigs/cluster-api-bootstrap-provider-kubeadm//config/default/?ref=${CABPK_VERSION}" > "${COMPONENTS_KUBEADM_GENERATED_FILE}"
echo "Generated ${COMPONENTS_KUBEADM_GENERATED_FILE}"

# Download Network Plugin (Calico) manifest
curl -sL https://docs.projectcalico.org/${CALICO_VERSION}/manifests/calico.yaml -o "${ADDON_GENERATED_FILE}"
echo "Downloaded ${ADDON_GENERATED_FILE}"

# Generate a single provider components file.
kustomize build "${SOURCE_DIR}/provider-components" | envsubst > "${PROVIDER_COMPONENTS_GENERATED_FILE}"
echo "Generated ${PROVIDER_COMPONENTS_GENERATED_FILE}"
echo "WARNING: ${PROVIDER_COMPONENTS_GENERATED_FILE} includes Vultr credentials"

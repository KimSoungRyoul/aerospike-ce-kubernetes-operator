#!/bin/bash
set -e

# Configuration directory
CONFIG_SRC="/configmap"
CONFIG_DST="/etc/aerospike"

echo "Aerospike CE Operator Init Container"
echo "======================================"

# Copy configuration files
echo "Copying aerospike.conf from ConfigMap..."
if [ -f "${CONFIG_SRC}/aerospike.conf" ]; then
    cp "${CONFIG_SRC}/aerospike.conf" "${CONFIG_DST}/aerospike.conf"
    echo "Configuration copied successfully."
else
    echo "ERROR: aerospike.conf not found in ConfigMap!"
    exit 1
fi

# Get pod information from environment
POD_NAME=${POD_NAME:-$(hostname)}
POD_IP=${POD_IP:-$(hostname -i 2>/dev/null || echo "127.0.0.1")}
NODE_IP=${NODE_IP:-""}

echo "Pod Name: ${POD_NAME}"
echo "Pod IP: ${POD_IP}"
echo "Node IP: ${NODE_IP}"

# Replace placeholders in config
if [ -n "${POD_IP}" ]; then
    sed -i "s/MY_POD_IP/${POD_IP}/g" "${CONFIG_DST}/aerospike.conf"
fi
if [ -n "${POD_NAME}" ]; then
    sed -i "s/MY_POD_NAME/${POD_NAME}/g" "${CONFIG_DST}/aerospike.conf"
fi
if [ -n "${NODE_IP}" ]; then
    sed -i "s/MY_NODE_IP/${NODE_IP}/g" "${CONFIG_DST}/aerospike.conf"
fi

# Resolve external address/port from per-pod Kubernetes Service.
# When EXTERNAL_SERVICE_TYPE is set, the operator created a LoadBalancer or
# NodePort Service for this pod. We query it to discover the externally
# reachable address and port, then substitute the placeholders that the
# config generator injected.
EXTERNAL_SERVICE_TYPE="${EXTERNAL_SERVICE_TYPE:-}"
POD_NAMESPACE="${POD_NAMESPACE:-default}"

if [ -n "${EXTERNAL_SERVICE_TYPE}" ]; then
    SVC_NAME="${POD_NAME}-pod"
    SA_TOKEN_PATH="/var/run/secrets/kubernetes.io/serviceaccount/token"
    CA_CERT="/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"

    if [ ! -f "${SA_TOKEN_PATH}" ]; then
        echo "ERROR: Service account token not found at ${SA_TOKEN_PATH}."
        echo "  The operator should set automountServiceAccountToken=true and create"
        echo "  RBAC for the pod service account. Cannot resolve external address."
        exit 1
    fi

    TOKEN=$(cat "${SA_TOKEN_PATH}")
    API_URL="https://kubernetes.default.svc/api/v1/namespaces/${POD_NAMESPACE}/services/${SVC_NAME}"

    echo "Resolving external endpoint from service ${SVC_NAME} (type=${EXTERNAL_SERVICE_TYPE})..."

    # Retry up to 60 seconds for service to be ready
    MAX_RETRIES=12
    RETRY_INTERVAL=5
    SVC_JSON=""
    for i in $(seq 1 ${MAX_RETRIES}); do
        SVC_JSON=$(curl -sf --cacert "${CA_CERT}" -H "Authorization: Bearer ${TOKEN}" "${API_URL}" 2>/dev/null) && break
        echo "  Waiting for service ${SVC_NAME} (attempt ${i}/${MAX_RETRIES})..."
        sleep ${RETRY_INTERVAL}
    done

    if [ -z "${SVC_JSON}" ]; then
        echo "ERROR: Could not fetch service ${SVC_NAME} after ${MAX_RETRIES} attempts."
        exit 1
    fi

    # extract_ingress_ip: parse the LB external IP from .status.loadBalancer.ingress[].ip
    # Uses a two-step grep to avoid matching unrelated "ip" fields (e.g. clusterIP).
    extract_ingress_ip() {
        local json="$1"
        local ingress_section
        ingress_section=$(echo "${json}" | grep -oE '"ingress":\s*\[[^]]*\]' 2>/dev/null) || true
        if [ -n "${ingress_section}" ]; then
            echo "${ingress_section}" | grep -oE '"ip":\s*"[^"]*"' | head -1 | sed 's/.*"\([0-9.]*\)".*/\1/'
        fi
    }

    # extract_ingress_hostname: parse the LB hostname from .status.loadBalancer.ingress[].hostname
    extract_ingress_hostname() {
        local json="$1"
        local ingress_section
        ingress_section=$(echo "${json}" | grep -oE '"ingress":\s*\[[^]]*\]' 2>/dev/null) || true
        if [ -n "${ingress_section}" ]; then
            echo "${ingress_section}" | grep -oE '"hostname":\s*"[^"]*"' | head -1 | sed 's/.*"hostname":[[:space:]]*"\([^"]*\)".*/\1/'
        fi
    }

    if [ "${EXTERNAL_SERVICE_TYPE}" = "LoadBalancer" ]; then
        # Wait for LoadBalancer IP assignment (can take up to 120s)
        EXTERNAL_ADDR=""
        for i in $(seq 1 24); do
            EXTERNAL_ADDR=$(extract_ingress_ip "${SVC_JSON}")
            if [ -z "${EXTERNAL_ADDR}" ]; then
                EXTERNAL_ADDR=$(extract_ingress_hostname "${SVC_JSON}")
            fi
            if [ -n "${EXTERNAL_ADDR}" ]; then
                break
            fi
            echo "  Waiting for LoadBalancer IP (attempt ${i}/24)..."
            sleep 5
            SVC_JSON=$(curl -sf --cacert "${CA_CERT}" -H "Authorization: Bearer ${TOKEN}" "${API_URL}" 2>/dev/null) || true
        done

        if [ -n "${EXTERNAL_ADDR}" ]; then
            echo "External address resolved: ${EXTERNAL_ADDR}"
            sed -i "s/MY_EXTERNAL_ADDRESS/${EXTERNAL_ADDR}/g" "${CONFIG_DST}/aerospike.conf"
        else
            echo "ERROR: LoadBalancer IP not assigned after 120 seconds."
            exit 1
        fi
    elif [ "${EXTERNAL_SERVICE_TYPE}" = "NodePort" ]; then
        EXTERNAL_PORT=$(echo "${SVC_JSON}" | grep -oE '"nodePort":\s*[0-9]+' | head -1 | grep -oE '[0-9]+$')
        if [ -n "${EXTERNAL_PORT}" ]; then
            echo "External port resolved: ${EXTERNAL_PORT}"
            sed -i "s/MY_EXTERNAL_PORT/${EXTERNAL_PORT}/g" "${CONFIG_DST}/aerospike.conf"
        else
            echo "ERROR: NodePort not found in service ${SVC_NAME}."
            exit 1
        fi
    fi
fi

# Helper: process volume operations (used by both WIPE and INIT)
process_volumes() {
    local label="$1"
    local volumes="$2"

    if [ -z "${volumes}" ]; then
        return
    fi

    echo "Processing ${label}..."
    IFS=',' read -ra VOLS <<< "${volumes}"
    for vol_spec in "${VOLS[@]}"; do
        IFS=':' read -ra PARTS <<< "${vol_spec}"
        method="${PARTS[0]}"
        path="${PARTS[1]}"

        case "${method}" in
            deleteFiles)
                echo "[${label}] Deleting files in ${path}..."
                rm -rf "${path:?}"/*
                ;;
            dd)
                echo "[${label}] Zeroing first 1MB of ${path}..."
                dd if=/dev/zero of="${path}" bs=1M count=1 conv=notrunc 2>/dev/null
                ;;
            blkdiscard)
                echo "[${label}] Discarding blocks on ${path}..."
                blkdiscard "${path}" 2>/dev/null || echo "blkdiscard failed for ${path}, continuing..."
                ;;
            headerCleanup)
                echo "[${label}] Cleaning Aerospike headers on ${path}..."
                dd if=/dev/zero of="${path}" bs=4096 count=1 conv=notrunc 2>/dev/null
                ;;
            blkdiscardWithHeaderCleanup)
                echo "[${label}] Discarding blocks and cleaning headers on ${path}..."
                blkdiscard "${path}" 2>/dev/null || echo "blkdiscard failed for ${path}, continuing..."
                dd if=/dev/zero of="${path}" bs=4096 count=1 conv=notrunc 2>/dev/null
                ;;
            *)
                echo "[${label}] Skipping ${path} (method: ${method})"
                ;;
        esac
    done
}

# Wipe dirty volumes (runs before init, only for volumes marked dirty)
WIPE_VOLUMES="${WIPE_VOLUMES:-}"
process_volumes "WIPE" "${WIPE_VOLUMES}"

# Volume initialization
INIT_VOLUMES="${INIT_VOLUMES:-}"
process_volumes "INIT" "${INIT_VOLUMES}"

echo "Init container completed successfully."

#!/bin/bash -E

CURDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
source "$CURDIR"/kubectl_retry.sh

ROOK_DEPLOY_TIMEOUT=${ROOK_DEPLOY_TIMEOUT:-6000}
#ROOK_PATH=${AKASH_ROOT:-${pwd}}
#ROOK_PATH=${ROOK_PATH}/_docs/rook

if [ -z "$ROOK_PATH" ]; then
	echo "ROOK_PATH is not set"
	exit 1
fi

rook_files=(
	"${ROOK_PATH}/crds.yaml"
	"${ROOK_PATH}/common.yaml"
	"${ROOK_PATH}/operator.yaml"
	"${ROOK_PATH}/cluster.yaml"
	"${ROOK_PATH}/toolbox.yaml"
	"${ROOK_PATH}/akash-nodes-pool.yaml"
	"${ROOK_PATH}/akash-deployments-pool.yaml"
	"${ROOK_PATH}/akash-nodes-storageclass.yaml"
	"${ROOK_PATH}/akash-deployments-storageclass.yaml"
)

for idx in "${!rook_files[@]}"; do
	if [ ! -f "${rook_files[idx]}" ]; then
		echo "required file ${rook_files[idx]} does not exist"
		exit 1
	fi
done

trap log_errors ERR

# log_errors is called on exit (see 'trap' above) and tries to provide
# sufficient information to debug deployment problems
function log_errors() {
	# enable verbose execution
	set -x

	kubectl get nodes
	kubectl -n rook-ceph get events
	kubectl -n rook-ceph describe pods
	kubectl -n rook-ceph logs -l app=rook-ceph-operator
	kubectl -n rook-ceph get CephClusters -oyaml
	kubectl -n rook-ceph get CephFilesystems -oyaml
	kubectl -n rook-ceph get CephBlockPools -oyaml

	# this function should not return, a fatal error was caught!
	exit 1
}

rook_version() {
	echo "${ROOK_VERSION#v}" | cut -d'.' -f"${1}"
}

function deploy_rook() {
	for idx in "${!rook_files[@]}"; do
		kubectl_retry apply -f "${rook_files[idx]}"
	done

	# Check if CephCluster is empty
	if ! kubectl_retry -n rook-ceph get cephclusters -oyaml | grep 'items: \[\]' &>/dev/null; then


		if [[ $(check_ceph_cluster_health) -ne 0 ]]; then
			echo ""
		else
			echo "CEPH cluster not in a healthy state (timeout)"
		fi
	fi

	# Check if CephFileSystem is empty
	if ! kubectl_retry -n rook-ceph get cephfilesystems -oyaml | grep 'items: \[\]' &>/dev/null; then
		check_mds_stat
	fi

	# Check if CephBlockPool is empty
	if ! kubectl_retry -n rook-ceph get cephblockpools -oyaml | grep 'items: \[\]' &>/dev/null; then
		check_rbd_stat ""
	fi
}

function teardown_rook() {
	for ((idx=${#rook_files[@]}-1 ; idx>=0 ; idx--)) ; do
		kubectl_retry delete -f "${rook_files[idx]}"
	done
}

function check_ceph_cluster_health() {
	for ((retry = 0; retry <= ROOK_DEPLOY_TIMEOUT; retry = retry + 5)); do
		CEPH_STATE=$(kubectl_retry -n rook-ceph get cephclusters -o jsonpath='{.items[0].status.state}')
		CEPH_HEALTH=$(kubectl_retry -n rook-ceph get cephclusters -o jsonpath='{.items[0].status.ceph.health}')
		echo "Checking CEPH cluster state: [$CEPH_STATE]"
		if [ "$CEPH_STATE" = "Created" ]; then
			if [ "$CEPH_HEALTH" = "HEALTH_OK" ]; then
				echo "The CEPH cluster health state [$CEPH_HEALTH]"
				break
			elif [ "$retry" -lt "$ROOK_DEPLOY_TIMEOUT" ]; then
				sleep 5
			fi
		fi
	done

	if [ "$retry" -gt "$ROOK_DEPLOY_TIMEOUT" ]; then
		return 1
	fi

	return 0
}

function check_mds_stat() {
	for ((retry = 0; retry <= ROOK_DEPLOY_TIMEOUT; retry = retry + 5)); do
		FS_NAME=$(kubectl_retry -n rook-ceph get cephfilesystems.ceph.rook.io -ojsonpath='{.items[0].metadata.name}')
		echo "Checking MDS ($FS_NAME) stats... ${retry}s" && sleep 5

		ACTIVE_COUNT=$(kubectl_retry -n rook-ceph get cephfilesystems myfs -ojsonpath='{.spec.metadataServer.activeCount}')

		ACTIVE_COUNT_NUM=$((ACTIVE_COUNT + 0))
		echo "MDS ($FS_NAME) active_count: [$ACTIVE_COUNT_NUM]"
		if ((ACTIVE_COUNT_NUM < 1)); then
			continue
		else
			if kubectl_retry -n rook-ceph get pod -l rook_file_system=myfs | grep Running &>/dev/null; then
				echo "Filesystem ($FS_NAME) is successfully created..."
				break
			fi
		fi
	done

	if [ "$retry" -gt "$ROOK_DEPLOY_TIMEOUT" ]; then
		echo "[Timeout] Failed to get ceph filesystem pods"
		return 1
	fi
	echo ""
}

function check_rbd_stat() {
	for ((retry = 0; retry <= ROOK_DEPLOY_TIMEOUT; retry = retry + 5)); do
		if [ -z "$1" ]; then
			RBD_POOL_NAME=$(kubectl_retry -n rook-ceph get cephblockpools -ojsonpath='{.items[0].metadata.name}')
		else
			RBD_POOL_NAME=$1
		fi
		echo "Checking RBD ($RBD_POOL_NAME) stats... ${retry}s" && sleep 5

		TOOLBOX_POD=$(kubectl_retry -n rook-ceph get pods -l app=rook-ceph-tools -o jsonpath='{.items[0].metadata.name}')
		TOOLBOX_POD_STATUS=$(kubectl_retry -n rook-ceph get pod "$TOOLBOX_POD" -ojsonpath='{.status.phase}')
		[[ "$TOOLBOX_POD_STATUS" != "Running" ]] && \
			{ echo "Toolbox POD ($TOOLBOX_POD) status: [$TOOLBOX_POD_STATUS]"; continue; }

		if kubectl_retry exec -n rook-ceph "$TOOLBOX_POD" -it -- rbd pool stats "$RBD_POOL_NAME" &>/dev/null; then
			echo "RBD ($RBD_POOL_NAME) is successfully created..."
			break
		fi
	done

	if [ "$retry" -gt "$ROOK_DEPLOY_TIMEOUT" ]; then
		echo "[Timeout] Failed to get RBD pool stats"
		return 1
	fi
	echo ""
}

case "${1:-}" in
deploy)
	deploy_rook
	;;
teardown)
	teardown_rook
	;;
health)
	check_ceph_cluster_health
	;;
*)
	echo " $0 [command]
Available Commands:
  deploy             Deploy a rook
  teardown           Teardown a rook
  health             Check cluster health
" >&2
	;;
esac

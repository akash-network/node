#!/bin/bash -e

CURDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
source "$CURDIR"/kubectl_retry.sh

#feature-gates for kube
K8S_FEATURE_GATES=${K8S_FEATURE_GATES:-"BlockVolume=true,CSIBlockVolume=true,VolumeSnapshotDataSource=true,ExpandCSIVolumes=true"}

VDISK_SIZE=64
VDISK_FILE="${AKASH_HOME}/disk1-${VDISK_SIZE}gb"

LOSETUP_RPM=https://www.rpmfind.net/linux/openmandriva/4.2/repository/x86_64/main/updates/util-linux-2.36.2-1-omv4002.x86_64.rpm

# configure minikube
KUBE_VERSION=${KUBE_VERSION:-"v1.22.1"}
CONTAINER_CMD=${CONTAINER_CMD:-"docker"}
MEMORY=${MEMORY:-"8192"}
CPUS=${CPUS:-"4"}

# detect if there is a minikube executable available already. If there is none,
# fallback to using /usr/local/bin/minikube, as that is where
# install_minikube() will place it too.
function detect_minikube() {
	if type minikube >/dev/null 2>&1; then
		result=$(command -v minikube)
	else
		# default if minikube is not available
		result='/usr/local/bin/minikube'
	fi

	echo "$result"
}

function detect_kubectl() {
	if type kubectl >/dev/null 2>&1; then
		command -v kubectl
		return
	fi
	# default if kubectl is not available
	echo '/usr/local/bin/kubectl'
}

minikube="$(detect_minikube)"

if [[ -z "$VM_DRIVER" ]]; then
	if command -v prlctl &> /dev/null ; then
		VM_DRIVER="parallels"
	elif command -v vboxmanage &> /dev/null; then
		VM_DRIVER="virtualbox"
	elif command -v virsh &> /dev/null; then
		VM_DRIVER="kvm2"
	else
		echo "no supported VM drivers found"
		exit 1
	fi
fi

case $VM_DRIVER in
parallels|kvm2)
	VDISK_FILE="${VDISK_FILE}.hdd"
	;;
virtualbox)
	VDISK_FILE="${VDISK_FILE}.vdi"
	;;
*)
	echo "unsupported VM_DRIVER=$VM_DRIVER. supported are parallels|virtualbox|kvm2"
	exit 1
	;;
esac

echo "using vm_driver=$VM_DRIVER"

DISK="sda1"
if [[ "${VM_DRIVER}" == "kvm2" ]]; then
	# use vda1 instead of sda1 when running with the libvirt driver
	DISK="vda1"
fi

# Storage providers and the default storage class is not needed for Ceph-CSI
# testing. In order to reduce resources and potential conflicts between storage
# plugins, disable them.
function disable_storage_addons() {
	# shellcheck disable=SC2154
	${minikube} addons disable default-storageclass &>/dev/null || true
	${minikube} addons disable storage-provisioner &>/dev/null || true
}

function wait_for_ssh() {
	local tries=100
	while ((tries > 0)); do
		if ${minikube} ssh echo connected &>/dev/null; then
			return 0
		fi
		tries=$((tries - 1))
		sleep 0.1
	done
	echo ERROR: ssh did not come up >&2
	exit 1
}

# minikube has the Busybox losetup, and that does not work with raw-block PVCs.
# Copy the host losetup executable and hope it works.
#
# See https://github.com/kubernetes/minikube/issues/8284
function minikube_losetup() {
	# scp should not ask for any confirmation
	pushd "$(pwd)"

	cd "$AKASH_DEVCACHE"

	rpm2cpio "${LOSETUP_RPM}" | cpio -ivdm ./sbin/losetup
	scp -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no -i "$(${minikube} ssh-key)" "${AKASH_DEVCACHE}/sbin/losetup" docker@"$(${minikube} ip)":losetup
	rm -f "${AKASH_DEVCACHE}/sbin/losetup"

	popd

	# replace /sbin/losetup symlink with the executable
	# shellcheck disable=SC2016
	${minikube} ssh 'sudo sh -c "rm -f "/usr/sbin/losetup" && cp ~docker/losetup /usr/sbin"'
}

function validate_container_cmd() {
	local cmd="${CONTAINER_CMD##* }"
	if [[ "${cmd}" == "docker" ]] || [[ "${cmd}" == "podman" ]]; then
		if ! command -v "${cmd}" &> /dev/null; then
			echo "'${cmd}' not found"
			exit 1
		fi
	else
		echo "'CONTAINER_CMD' should be either docker or podman and not '${cmd}'"
		exit 1
	fi
}

function copy_image_to_cluster() {
	local build_image=$1
	local final_image=$2
	validate_container_cmd
	if [ -z "$(${CONTAINER_CMD} images -q "${build_image}")" ]; then
		${CONTAINER_CMD} pull "${build_image}"
	fi
	if [[ "${VM_DRIVER}" == "none" ]]; then
		${CONTAINER_CMD} tag "${build_image}" "${final_image}"
		return
	fi

	# "minikube ssh" fails to read the image, so use standard ssh instead
	${CONTAINER_CMD} save "${build_image}" | \
		ssh \
			-o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no \
			-i "$(${minikube} ssh-key)" -l docker \
			"$(${minikube} ip)" docker image load
}

case "${1:-}" in
up)
	disable_storage_addons

	echo "starting minikube with kubeadm bootstrapper"

	# shellcheck disable=SC2086
	${minikube} start --memory="${MEMORY}" --cpus="${CPUS}" --driver="${VM_DRIVER}" #-b kubeadm --kubernetes-version="${KUBE_VERSION}" --feature-gates="${K8S_FEATURE_GATES}"
	# create a link so the default dataDirHostPath will work for this environment
	if [[ "${VM_DRIVER}" != "none" ]]; then
		wait_for_ssh
		# shellcheck disable=SC2086
		${minikube} ssh "sudo mkdir -p /mnt/${DISK}/var/lib/rook;sudo ln -s /mnt/${DISK}/var/lib/rook /var/lib/rook"
		minikube_losetup
	fi

	${minikube} stop

	case $VM_DRIVER in
	parallels)
		prl_disk_tool create --hdd "$VDISK_FILE" --size ${VDISK_SIZE}G --expanding
		prlctl set "minikube" --device-add hdd --image "$VDISK_FILE"
		;;

	virtualbox)
		# virtualbox is dumb and does not except size in gigabytes
		vboxmanage createmedium disk --filename "$VDISK_FILE" --size $((VDISK_SIZE * 1024)) --format vdi --variant Standard
		vboxmanage storageattach minikube --storagectl "SATA" --port 1 --type hdd --medium "$VDISK_FILE"
		;;
	kvm2)
		sudo -S qemu-img create -f raw "$VDISK_FILE" ${VDISK_SIZE}G
		virsh -c qemu:///system attach-disk minikube --source "$VDISK_FILE" --target vdb --cache none
		virsh -c qemu:///system reboot --domain minikube
		;;
	esac

	${minikube} start --memory="${MEMORY}" --cpus="${CPUS}" --driver="${VM_DRIVER}" #-b kubeadm --kubernetes-version="${KUBE_VERSION}" --driver="${VM_DRIVER}" --feature-gates="${K8S_FEATURE_GATES}"
	${minikube} kubectl -- cluster-info
	;;

down)
	${minikube} stop
	;;

ssh)
	echo "connecting to minikube"
	${minikube} ssh
	;;

clean)
	${minikube} delete

	case "$VM_DRIVER" in
	parallels)
		prlctl delete minikube || true
		;;
	virtualbox)
		vboxmanage unregistervm minikube --delete || true
		;;
	esac

	rm -rf "$VDISK_FILE"

	;;

akash-setup)
	kubectl_retry apply -f "$AKASH_ROOT/_docs/kustomize/networking/"

	kubectl_retry label nodes minikube akash.network/storageclasses=beta2
	kubectl_retry label nodes minikube akash.network/role=ingress

	kubectl_retry apply -f "${AKASH_ROOT}/pkg/apis/akash.network/crd.yaml"

	kubectl kustomize "${AKASH_ROOT}/_docs/kustomize/akash-services/" | kubectl_retry apply -f-

	;;
deploy-rook)
	echo "deploy rook"
	"$CURDIR"/rook.sh deploy
	;;
*)
	echo "$0 [command]
Available Commands:
  up                Starts a local kubernetes cluster and prepare disk for rook
  clean             Delete a running local kubernetes cluster
  ssh               Log into or run a command on a minikube machine with SSH
  deploy-rook       Deploy rook to minikube
" >&2
	;;
esac

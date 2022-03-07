# Lets begin

At Akash we use Rook/Ceph couple to provider dynamic volumes provisioning
- Ceph - distributed filesystem
- Rook - Kubernetes operator to control/provision Ceph

In the guide **Rook** means Rook/Ceph couple.
This guide is focused on Rook setup and usage for Akash provider. To get familiar with Rook concepts refer to the [official documentation](https://rook.github.io/docs/rook/v1.8/)
Configuration is based on Rook V1.8

## Prerequisites
1. Helm charts referenced in the guide are present [here](https://github.com/ovrclk/helm-charts).
2. Deploy and teardown are done by using bash script. Work on the helm chart is being done. Up until helm chart is ready this guide will be focused on creating test cluster only         
Meanwhile examples referenced in the guide can be found [here](TBD)
3. We will prevent cluster from using master nodes.
4. We will be utilizing all available devices on each used node
5. AKASH_ROOT environment variable points to local copy of [akash codebase](https://github.com/ovrclk/akash)

## Configuration

### Kubernetes nodes

In this example we are focusing on making slight adjustments to the `cluster.yaml` only. All other files must remain unchanged.
We recommend to exclude all master nodes from any workloads including persistent storage

1. get list of nodes in the cluster
```shell
kubectl get nodes -ojson | jq -r '.items[].metadata.labels."kubernetes.io/hostname"'
k8s-master.edgenet-1.ewr1 # excluded, it is not recommended to use master nodes
k8s-node-0.edgenet-1.ewr1
k8s-node-1.edgenet-1.ewr1
```

2. Label nodes
Each node carrying workloads should have `akash.network/storageclasses` label set. Further purpose of the label to allow
different nodes deploying different storage classes.
For though now value of the label should be same for all nodes!

Value of the label is period separated list of supported storage classes.
For example:
    cluster supports `default` and `beta2` storage classes and label will look like following `akash.network/storageclasses=default.beta2`

In example below all nodes except master are labeled with `akash.network/storageclasses=default.beta2` 
```shell
kubectl label nodes k8s-node-0.edgenet-1.ewr1 akash.network/storageclasses=default.beta2 --overwrite
kubectl label nodes k8s-node-0.edgenet-1.ewr1 akash.network/storageclasses=default.beta2 --overwrite
```

3. replace `nodes:` in `cluster.yaml` with queried node above
```yaml
nodes:
  - name: k8s-node-0.edgenet-1.ewr1
  - name: k8s-node-1.edgenet-1.ewr1
```

### Node block devices
Rook can use only blank devices to build the cluster. If any sort of filesystem present on the disk it will be ignored.
In this example we won't limit what devices can be used and allow rook to pick all blank devices on the node.

### Configuring provider pricing
#### Environment variable or flag
Use either `AKASH_BID_PRICE_STORAGE_SCALE` environment variable or `--bid-pricing-storage-scale` flag. Flag takes higher priority if set. 
Format of the value is same for both env variable and flag and defined by the template `<ephemeral scale>[,<class>=scale][,<class>=scale]`.
We will use environment variable as detailed example

```shell
# provider accepts orders with ephemeral storage only
AKASH_BID_PRICE_STORAGE_SCALE=0.001

# provider accepts orders with ephemeral storage and beta2 storage class
AKASH_BID_PRICE_STORAGE_SCALE=0.0001,beta2=0.002

# provider accepts orders with ephemeral storage, default, beta2
# for this provider default storage class is an alias to the beta2. default class must be specified in order to bid on orders with default storage class
# even tho in this example it uses same price as beta2  
AKASH_BID_PRICE_STORAGE_SCALE=0.0001,default=0.002,beta2=0.002
```

#### Pricing script
Following [script](https://github.com/ovrclk/akash/blob/master/script/usd_pricing_oracle.sh) can be used for dynamic price changes.
Storage classes are defined on lines 47..51. Comment or remove unsupported storage classes and change scale accordingly.

## Deploying test cluster
```shell
# deploy takes around 10m
ROOK_PATH=${AKASH_ROOT}/_docs/rook/test ./scripts/rook.sh deploy 
```

## Check cluster health
```shell
ROOK_PATH=${AKASH_ROOT}/_docs/rook/test ./scripts/rook.sh health
```

### Debugging ceph via tools

If ceph health has a warning it further default can be extracted to find a root cause
shell into toolbox pod
then
```shell
# list crashes (new entities have asterisk in the NEW column)
ceph crash ls
ID                                                                ENTITY  NEW
2022-02-22T14:23:00.759502Z_daf79031-9965-48b9-b1f0-5344d49127ad  osd.3

# get details of the crash
ceph crash info 2022-02-22T14:23:00.759502Z_daf79031-9965-48b9-b1f0-5344d49127ad
{
    "archived": "2022-02-22 14:43:32.196451",
    "assert_condition": "r == 0",
    "assert_file": "/home/jenkins-build/build/workspace/ceph-build/ARCH/x86_64/AVAILABLE_ARCH/x86_64/AVAILABLE_DIST/centos8/DIST/centos8/MACHINE_SIZE/gigantic/release/16.2.5/rpm/el8/BUILD/ceph-16.2.5/src/os/bluestore/BlueFS.cc",
    "assert_func": "int64_t BlueFS::_read(BlueFS::FileReader*, uint64_t, size_t, ceph::bufferlist*, char*)",
    "assert_line": 2032,
    "assert_msg": "/home/jenkins-build/build/workspace/ceph-build/ARCH/x86_64/AVAILABLE_ARCH/x86_64/AVAILABLE_DIST/centos8/DIST/centos8/MACHINE_SIZE/gigantic/release/16.2.5/rpm/el8/BUILD/ceph-16.2.5/src/os/bluestore/BlueFS.cc: In function 'int64_t BlueFS::_read(BlueFS::FileReader*, uint64_t, size_t, ceph::bufferlist*, char*)' thread 7fea3ecaa080 time 2022-02-22T14:23:00.752201+0000\n/home/jenkins-build/build/workspace/ceph-build/ARCH/x86_64/AVAILABLE_ARCH/x86_64/AVAILABLE_DIST/centos8/DIST/centos8/MACHINE_SIZE/gigantic/release/16.2.5/rpm/el8/BUILD/ceph-16.2.5/src/os/bluestore/BlueFS.cc: 2032: FAILED ceph_assert(r == 0)\n",
    "assert_thread_name": "ceph-osd",
    "backtrace": [
        "/lib64/libpthread.so.0(+0x12b20) [0x7fea3c80ab20]",
        "gsignal()",
        "abort()",
        "(ceph::__ceph_assert_fail(char const*, char const*, int, char const*)+0x1a9) [0x55acb441df0b]",
        "/usr/bin/ceph-osd(+0x56a0d4) [0x55acb441e0d4]",
        "(BlueFS::_read(BlueFS::FileReader*, unsigned long, unsigned long, ceph::buffer::v15_2_0::list*, char*)+0x7af) [0x55acb4b0b3ff]",
        "(BlueFS::_replay(bool, bool)+0x3c6) [0x55acb4b25346]",
        "(BlueFS::mount()+0x120) [0x55acb4b29af0]",
        "(BlueStore::_open_bluefs(bool, bool)+0x94) [0x55acb4a089a4]",
        "(BlueStore::_prepare_db_environment(bool, bool, std::__cxx11::basic_string<char, std::char_traits<char>, std::allocator<char> >*, std::__cxx11::basic_string<char, std::char_traits<char>, std::allocator<char> >*)+0x6e1) [0x55acb4a09b01]",
        "(BlueStore::_open_db(bool, bool, bool)+0x15f) [0x55acb4a0ae2f]",
        "(BlueStore::mkfs()+0x1136) [0x55acb4a79d06]",
        "(OSD::mkfs(ceph::common::CephContext*, ObjectStore*, uuid_d, int, std::__cxx11::basic_string<char, std::char_traits<char>, std::allocator<char> >)+0x1af) [0x55acb44fbabf]",
        "main()",
        "__libc_start_main()",
        "_start()"
    ],
    "ceph_version": "16.2.5",
    "crash_id": "2022-02-22T14:23:00.759502Z_daf79031-9965-48b9-b1f0-5344d49127ad",
    "entity_name": "osd.3",
    "os_id": "centos",
    "os_name": "CentOS Linux",
    "os_version": "8",
    "os_version_id": "8",
    "process_name": "ceph-osd",
    "stack_sig": "e853aee02f422bd61d51579c300bd25bef66fcc02d2e24f25dd07c476e8f29c6",
    "timestamp": "2022-02-22T14:23:00.759502Z",
    "utsname_hostname": "rook-ceph-osd-prepare-node-5.edgenet-3.lumen-2szjh",
    "utsname_machine": "x86_64",
    "utsname_release": "5.4.0-99-generic",
    "utsname_sysname": "Linux",
    "utsname_version": "#112-Ubuntu SMP Thu Feb 3 13:50:55 UTC 2022"
}

# if root cause has been identified and fixed the message should be archived to move cluster into HEALTH_OK state
ceph crash archive 2022-02-22T14:23:00.759502Z_daf79031-9965-48b9-b1f0-5344d49127ad

# all messages can be archived at once
ceph crash archive-all
```

## Inventory operator

Up until this point we were working on adjustments to the Kubernetes cluster itself

1. Make sure akash provider is installed and running. Follow installation [guide](https://github.com/ovrclk/helm-charts#akash-provider-install) if not
2. Install [inventory operator](https://github.com/ovrclk/helm-charts#akash-inventory-operator-optional---for-persistent-storage)

## Teardown

```shell
ROOK_PATH=${AKASH_ROOT}/_docs/rook/test ./scripts/rook.sh teardown
```

### Zapping Devices

Disks on nodes used by Rook for osds can be reset to a usable state with the following method:

Execute following script on each node that participated in rook cluster with list of comma separated devices needs to be wiped.
For example devices `sda`,`sdc`, `sdg`
```shell
./zap.sh sda,sdc,sdg
```

#### zap.sh script
```shell
#!/usr/bin/env bash

if [ "$#" -ne 1 ]; then
	echo "Illegal number of parameters"
fi

# Zap the disk to a fresh, usable state (zap-all is important, b/c MBR has to be clean)
IFS=","
for disk in ${$1}; do
	dev=/dev/${disk}
	sgdisk --zap-all /dev/${disk}
	
	if [[ $(cat /sys/block/${disk}/queue/rotational) == "1" ]]; then
		# Clean hdds with dd
		dd if=/dev/zero of=${dev} bs=1M count=100 oflag=direct,dsync
	else
		# Clean disks such as ssd with blkdiscard instead of dd
		blkdiscard ${dev}
	fi
	
	# Inform the OS of partition table changes
partprobe ${dev}
done

# These steps only have to be run once on each node
# If rook sets up osds using ceph-volume, teardown leaves some devices mapped that lock the disks.
ls /dev/mapper/ceph-* | xargs -I% -- dmsetup remove %

# ceph-volume setup can leave ceph-<UUID> directories in /dev and /dev/mapper (unnecessary clutter)
rm -rf /dev/ceph-*
rm -rf /dev/mapper/ceph--*
```

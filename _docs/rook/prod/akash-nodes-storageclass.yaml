apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: akash-nodes
  labels:
    akash.network: "true"
provisioner: rook-ceph.rbd.csi.ceph.com
parameters:
  pool: akash-nodes

  # The value of "clusterID" MUST be the same as the one in which your rook cluster exist
  clusterID: rook-ceph

  # RBD image format. Defaults to "2".
  imageFormat: "2"

  # RBD image features. Available for imageFormat: "2". CSI RBD currently supports only `layering` feature.
  imageFeatures: layering

  # Specify the filesystem type of the volume. If not specified, it will use `ext4`.
  csi.storage.k8s.io/fstype: ext4

  csi.storage.k8s.io/provisioner-secret-name: rook-csi-rbd-provisioner
  csi.storage.k8s.io/provisioner-secret-namespace: rook-ceph # namespace:cluster
  csi.storage.k8s.io/controller-expand-secret-name: rook-csi-rbd-provisioner
  csi.storage.k8s.io/controller-expand-secret-namespace: rook-ceph # namespace:cluster
  csi.storage.k8s.io/node-stage-secret-name: rook-csi-rbd-node
  csi.storage.k8s.io/node-stage-secret-namespace: rook-ceph # namespace:cluster
reclaimPolicy: Retain

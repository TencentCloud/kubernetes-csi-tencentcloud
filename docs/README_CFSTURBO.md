# kubernetes-csi-tencentcloud

`kubernetes-csi-tencentcloud` CFS plugins implement interface of [CSI](https://github.com/container-storage-interface/spec). It can enable your Container Orchestrator to use Tencent [Cloud File Storage Turbo](https://cloud.tencent.com/product/cfs).

## Features

* **Mount Protocol** - currently, we support nfs v3 protocol.
* **Static Provisioning** - firstly, create a CFS filesystem on tencent cloud manually, then mount it inside container.
* **Mount options** - mount options can be specified in `PersistentVolume` to define how the volume should be mounted.

## CFS CSI Driver on Kubernetes

**Requirements:**

* Kubernetes v1.14.x+
* kube-apiserver and kubelet need `--allow-privileged=true` (for v1.15.x+, kubelet defaults to set `--allow-privileged` to true)
* feature gates `CSINodeInfo=true,CSIDriverRegistry=true`

### RBAC

```yaml
kubectl apply -f  deploy/cfsturbo/kubernetes/csi-node-rbac.yaml
```

### Node Plugin

```yaml
kubectl apply -f  deploy/cfsturbo/kubernetes/csidriver.yaml
kubectl apply -f  deploy/cfsturbo/kubernetes/csi-node.yaml
```

### Examples

#### Static Volume Provisioning

**Note**: `volumeHandle` in PV must be unique, use pv name is better.

```yaml
kubectl create -f deploy/cfsturbo/examples/static-allinone.yaml
```

## PersistentVolume parameters

### NFS protocol

* proto: Required. Support `nfs` or `lustre`, you should install kernel module in node before use `lustre` protocol, see https://cloud.tencent.com/document/product/582/54765.
* host: Required. NFS host like `10.0.0.112`
* fsid: Required. CFS instance's fsid
* path: Optional. NFS subpath, default is `/`
* options: Optional. Mount options for CFS Turbo

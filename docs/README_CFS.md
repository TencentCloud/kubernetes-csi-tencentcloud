# kubernetes-csi-tencentcloud

`kubernetes-csi-tencentloud` CFS plugins implement interface of [CSI](https://github.com/container-storage-interface/spec). It can enable your Container Orchestrator use Tencent [Cloud File Storage](https://cloud.tencent.com/product/cfs).

## Features
* **Static Provisioning** - firstly, create a CFS filesystem on tencent cloud manually; then mount it inside container
* **Dynamic Provisioning** - use PVC to request the Kuberenetes to create a CFS filesystem on behalf of user and consumes the filesystem from inside container
* **Mount options ** - mount options can be specified in storageclass to define how the volume should be mounted

## CFS CSI Driver on Kubernetes

**Requirements:**

* Kubernetes v1.13.x+
* kube-apiserver and kubelet need `--allow-privileged=true` (for v1.15.x+, kubelet defaults to set `--allow-privileged` to true)
* feature gates`CSINodeInfo=true,CSIDriverRegistry=true`

### tencentcloud yunapi secret

**Note**： If you use tke(Tencent Kubernetes Engine), you may not add `TENCENTCLOUD_API_SECRET_ID` and `TENCENTCLOUD_API_SECRET_KEY`.

```yaml
# deploy/cfs/kubernetes/csi-provisioner-cfsplugin.yaml
...
- name: NODE_ID
    valueFrom:
    fieldRef:
        fieldPath: spec.nodeName
- name: CSI_ENDPOINT
    value: unix://plugin/csi.sock
- name: TENCENTCLOUD_API_SECRET_ID
    value: AKIDxxxxxxxxxxxxxxxxxxx
- name: TENCENTCLOUD_API_SECRET_KEY
    value: xxxxxxxxxxxxxxxxxxxxxxxx
...
```

### rbac

```yaml
kubectl apply -f  deploy/cfs/kubernetes/csi-cfs-rbac.yaml
```

### controller,node plugin

**If your k8s version >= 1.14**
```yaml
kubectl apply -f  deploy/cfs/kubernetes/csi-cfs-csidriver.yaml
kubectl apply -f  deploy/cfs/kubernetes/csi-nodeplugin-cfsplugin.yaml
kubectl apply -f  deploy/cfs/kubernetes/csi-provisioner-cfsplugin.yaml
```
**If your k8s version < 1.14**
```yaml
kubectl apply -f  deploy/cfs/kubernetes/csi-attacher-cfsplugin.yaml
kubectl apply -f  deploy/cfs/kubernetes/csi-nodeplugin-cfsplugin.yaml
kubectl apply -f  deploy/cfs/kubernetes/csi-provisioner-cfsplugin.yaml
```

### examples

#### Dynamic Volume Provisioning

```yaml
# deploy/cfs/examples/dynamic-provison-allinone.yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: cfsauto
parameters:
# first you must modify vpcid and subnetid in storageclass parameters
  vpcid: vpc-xxxxxxxx
  subnetid: subnet-xxxxxxxx
provisioner: com.tencent.cloud.csi.cfs

kubectl create -f deploy/cfs/examples/dynamic-provison-allinone.yaml
```

#### Static Volume Provisioning

**Note**: `volumeHandle` in PV must be unique.

```yaml
kubectl create -f deploy/cfs/examples/static-allinone.yaml
```

## StorageClass parameters

* vpcid: Required. CFS csi plugin support create cfs in a vpc.
* subnetid: Required. `subnetid` must belong to `vpcid`.
* zone: select your CFS zone, like `ap-guangzhou-3`.
* pgroupid: select a pgroup which you created in CFS console, default is `pgroupbasic`.
* storagetype: default is `SD` currently.

## PersistentVolume parameters

* host: Required. NFS host like `10.0.0.112`.
* path: NFS path default is `/`.
* vers: NFS version, support `3` and `4`, default is `4`.
* options: mount options for NFS.

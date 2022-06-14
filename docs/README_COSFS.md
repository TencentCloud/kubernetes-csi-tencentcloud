# cos

This plugin is able to mount TencentCloud COS buckets as filesystem to workloads.

**Static Provisioning:** create a COS bucket on tencent cloud manually, then mount it inside a container

## Building

Build cos images:

```sh
make cos
```

## Deploy with Kubernetes

**Requires Kubernetes 1.14+**

Your Kubernetes cluster must allow privileged pods (i.e. `--allow-privileged` flag must be set to true for both the API
server and the kubelet, and for v1.15.x+, kubelet defaults to set `--allow-privileged` to true). Moreover, as stated in
the [mount propagation docs](https://kubernetes.io/docs/concepts/storage/volumes/#mount-propagation), the Docker daemon
of the cluster nodes must allow shared mounts.

**Deploy COS:**

* If your k8s version >= 1.20

```sh
kubectl apply -f deploy/cosfs/kubernetes/csidriver-new.yaml
kubectl apply -f deploy/cosfs/kubernetes/csi-launcher-new.yaml
kubectl apply -f deploy/cosfs/kubernetes/csi-node-rbac.yaml
kubectl apply -f deploy/cosfs/kubernetes/csi-node-new.yaml
```

* If your k8s version = 1.18

```sh
kubectl apply -f deploy/cosfs/kubernetes/csidriver-old.yaml
kubectl apply -f deploy/cosfs/kubernetes/csi-launcher-new.yaml
kubectl apply -f deploy/cosfs/kubernetes/csi-node-rbac.yaml
kubectl apply -f deploy/cosfs/kubernetes/csi-node-new.yaml
```

* If your k8s version >= 1.14 && < 1.18

```sh
kubectl apply -f deploy/cosfs/kubernetes/csidriver-old.yaml
kubectl apply -f deploy/cosfs/kubernetes/csi-launcher-old.yaml
kubectl apply -f deploy/cosfs/kubernetes/csi-node-rbac.yaml
kubectl apply -f deploy/cosfs/kubernetes/csi-node-old.yaml
```

## Secret

Secret are required for mount COS bucket:
https://cloud.tencent.com/document/product/436/6883

You can create a secret use this command:

```sh
kubectl create secret generic csi-cos-secret -n kube-system --from-literal=SecretId=AKIDjustfortest --from-literal=SecretKey=justfortest
```

Or apply the secret yaml file:

```yaml
kubectl apply -f deploy/cosfs/examples/secret.yaml
```

## PersistentVolume parameters

* url: required, cos bucket url, eg: http://cos.ap-guangzhou.myqcloud.com
* bucket: required, cos bucket name 
* path: optional, cos bucket mount path, default `/`
* mounter: optional, cos mounter, support [cosfs|goosefs-lite], default `cosfs`
* dbglevel: optional, log level for mounter `cosfs`, support [dbg|info|warn|err|crit]
* additional_args: optional, mount options for mounter `cosfs`, eg `-oensure_diskfree=20480 -oallow_other`
* core_site: optional, config for mounter `goosefs-lite`, eg `fs.cosn.maxRetries=100, fs.cosn.retry.interval.seconds=4`
* goosefs_lite: optional, config for mounter `goosefs-lite`, eg `goosefs.fuse.umount.timeout=110000`

> cosfs pv example: deploy/cosfs/examples/cosfs/pv.yaml  
> cosfs additional_args: https://cloud.tencent.com/document/product/436/6883#.E5.B8.B8.E7.94.A8.E6.8C.82.E8.BD.BD.E9.80.89.E9.A1.B9  

> goosefs-lite pv example: deploy/cosfs/examples/goosefs-lite/pv.yaml  
> goosefs-lite core_site and goosefs_lite: https://tcloud-doc.isd.com/document/product/1424/73687?!preview

## Examples

All example yaml files can be found under [deploy/cosfs/examples/](/deploy/cosfs/examples).
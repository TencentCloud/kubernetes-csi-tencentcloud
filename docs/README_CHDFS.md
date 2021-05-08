# kubernetes-csi-chdfs-tencentcloud

This plugin is able to mount TencentCloud CHDFS filesystem to workloads. It only supports static provisioning now.

* **Static Provisioning** - firstly, create a CHDFS on tencent cloud manually; then mount it inside container.

## Building

CHDFS CSI plugin can be compiled in a form of a binary file or in a form of a Docker image. When compiled as an image, it's stored in the local Docker image store.

Building Docker image:

```bash
docker build -t yourimagename -f Dockerfile.chdfs .
```


## Deployment with Kubernetes

Requires Kubernetes 1.14+

Your Kubernetes cluster must allow privileged pods (i.e. `--allow-privileged` flag must be set to true for both the API server and the kubelet, and for v1.15.x+, kubelet defaults to set `--allow-privileged` to true). Moreover, as stated in the [mount propagation docs](https://kubernetes.io/docs/concepts/storage/volumes/#mount-propagation), the Docker daemon of the cluster nodes must allow shared mounts.

YAML manifests are located in [deploy/kubernetes](/deploy/kubernetes). Those manifests deploy service accounts, cluster roles and cluster role bindings.

**Deploy CSI external components:**

```bash
kubectl create -f deploy/kubernetes/chdfsexternal.yaml
```

Deploys deployment of external sidecar containers for CHDFS CSI driver.

**Deploy CHDFS launcher:**

```bash
kubectl create -f deploy/kubernetes/chdfslauncher.yaml
```

Deploys a daemon set with single container: launcher.

**Deploy CHDFS CSI driver and related RBACs:**

```bash
kubectl create -f deploy/kubernetes/chdfsplugin.yaml
kubectl create -f deploy/kubernetes/rbac.yaml
```

Deploys a daemon set with two containers: CSI driver-registrar and the CHDFS CSI driver.

## Verifying the deployment in Kubernetes

After successfully completing the steps above, you should see output similar to this:

```bash
$ kubectl get po
NAME                                                            READY   STATUS    RESTARTS   AGE
kube-system   csi-chdfslauncher-xwrcx                           1/1     Running   0          119m
kube-system   csi-chdfsplugin-external-runner-fc44789df-bf62p   4/4     Running   0          119m
kube-system   csi-chdfsplugin-js6mx                             2/2     Running   0          119m
```

You can try deploying a demo pod from [deploy/example/](/deploy/example) to test the deployment further.

## Test CHDFS plugins with Kubernetes 1.16

All yaml files referenced in this doc can be found under [deploy/example/](/deploy/example)

## Requirement

Secrets are required for mount CHDFS:

First you can create a secret use this command:
```
kubectl create secret generic csi-tencentcloud -n kube-system  --from-literal=SecretId=AKIDjustfortest --from-literal=SecretKey=justfortest
```

Then you can find this encoded secret, like this:

```yaml
apiVersion: v1
kind: Secret
type: Opaque
metadata:
  # Replaced by your secret name.
  name: csi-tencentcloud
  # Replaced by your secret namespace.
  namespace: kube-system
data:
  # Replaced by your temporary secret file content. You can generate a temporary secret key with these docs:
  # Note: The value must be encoded by base64.
  SecretId: VWVEJxRk5Fb0JGbDA4M...(base64 encode)
  SecretKey: Qa3p4ZTVCMFlQek...(base64 encode)
```

## Create PV and PVC

Currently CHDFS CSI Driver only supports static provisioning, so you need to create a PV first:

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: pv-chdfs
spec:
  accessModes:
  - ReadWriteMany
  capacity:
    storage: 1Gi
  csi:
    driver: com.tencent.cloud.csi.chdfs
    # Specify a unique volumeHandle, which is your file system ID.
    volumeHandle: XXXX
    volumeAttributes:
      # Allow other users to access [true/false].
      allowother: "true"
      # Any modification of memory will be synchronized to CHDFS in real time [true/false].
      sync: "fasle"
      # Display details of fuse interface calls [true/false].
      debug: "true"
      # The name of configMap.
      configmapname: "chdfs-config"
      # The namespace of configMap.
      configmapnamespaces: "kube-system"
```

In general, you only need to sepcify the file system ID you want to use inside `volumeAttributes`.
To specify the options for the `chdfs-fuse` command, you can store them in a `configMap`, here is a simple example:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: chdfs-config
  namespace: kube-system
data:
  chdfs: |
    [proxy]
    url="http://XXXX-XXXX.chdfs.ap-shanghai.myqcloud.com"

    [client]
    mount-point="XXXX-XXXX"
    renew-session-lease-time-sec=10

    [cache]
    update-sts-time-sec=30
    cos-client-timeout-sec=5
    inode-attr-expired-time-sec=30

    [cache.read]
    block-expired-time-sec=10
    max-block-num=256
    read-ahead-block-num=15
    max-cos-load-qps=1024
    load-thread-num=128
    select-thread-num=64
    rand-read=false

    [cache.write]
    max-mem-table-range-num=32
    max-mem-table-size-mb=64
    max-cos-flush-qps=256
    flush-thread-num=128
    commit-queue-len=100
    max-commit-heap-size=500
    auto-merge=true
    auto-sync=false
    auto-sync-time-ms=1000

    [log.file]
    filename="/log/chdfs.log"
    log-rotate=true
    max-size=2000
    max-days=7
    max-backups=100
```
you only need to sepcify the mount point inside `url` and `mount-point`.

Then you should see output similar to this:

```bash
$ kubectl get pv
NAME       CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS      CLAIM              STORAGECLASS   REASON    AGE
pv-chdfs   1Gi        RWX            Retain           Bound       default/pvc-chdfs                           4h10m
```

Now you can create a PVC to use the PV above:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
    name: pvc-chdfs
spec:
  accessModes:
  - ReadWriteMany
  resources:
    requests:
      storage: 1Gi
  # You can specify the pv name manually or just let kubernetes to bind the pv and pvc.
  # volumeName: pv-chdfs
  # Currently chdfs only supports static provisioning, the StorageClass name should be empty.
  storageClassName: ""
```

Then you should see that the PVC is already bound to the PV:

```bash
$ kubectl get pvc
NAME        STATUS    VOLUME     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
pvc-chdfs   Bound     pv-chdfs   12Gi       RWX                           4h12m
```

## Create a Pod to use the PVC

You can create a Pod to test the PVC:

```yaml
apiVersion: v1
kind: Pod
metadata:
    name: pod-chdfs
spec:
  containers:
  - name: pod-chdfs
    command: ["tail", "-f", "/etc/hosts"]
    image: "centos:latest"
    volumeMounts:
    - mountPath: /data
      name: chdfs
    resources:
      requests:
        memory: "50Mi"
        cpu: "0.1"
  volumes:
  - name: chdfs
    persistentVolumeClaim:
      # Replaced by your pvc name.
      claimName: pvc-chdfs
```

The Pod should work properly:

```bash
$ kubectl get po
NAME        READY     STATUS    RESTARTS   AGE
pod-chdfs   1/1       Running   0          2m34s
```

# kubernetes-csi-tencentcloud

This plugin is able to mount TencentCloud COS buckets to workloads.

* **Static Provisioning** - firstly, create a COS on tencent cloud manually; then mount it inside container
* **Mount options ** - mount options can be specified in storageclass to define how the volume should be mounted

## Building

COS CSI plugin can be compiled in a form of a binary file or in a form of a Docker image. When compiled as an image, it's stored in the local Docker image store.

Building Docker image:

```bash
docker build --network host -t yourimagename -f Dockerfile.cosfs .

# launcher is a component which launch cosfs
docker build --network host -t yourimagename -f Dockerfile.launcher .
```

## Configuration

**Available command line arguments:**

Option | Default value | Description
------ | ------------- | -----------
`--endpoint` | `unix://csi/csi.sock` | CSI endpoint, must be a UNIX socket
`--drivername` | `com.tencent.cloud.csi.cosfs` | name of the driver
`--nodeid` | _empty_ | This node's ID

## Deployment with Kubernetes

Requires Kubernetes 1.10+

If you want use cos csi plugin in Kubernetes 1.12, you should update your kubelet config add 
```
--feature-gates=KubeletPluginsWatcher=false
```

Your Kubernetes cluster must allow privileged pods (i.e. `--allow-privileged` flag must be set to true for both the API server and the kubelet, and for v1.15.x+, kubelet defaults to set `--allow-privileged` to true). Moreover, as stated in the [mount propagation docs](https://kubernetes.io/docs/concepts/storage/volumes/#mount-propagation), the Docker daemon of the cluster nodes must allow shared mounts.

YAML manifests are located in [deploy/cosfs/kubernetes](/deploy/cosfs/kubernetes). Those manifests deploy service accounts, cluster roles and cluster role bindings.

**Deploy CSI external components:**
* If your k8s version >= 1.14
```bash
kubectl create -f deploy/cosfs/kubernetes/coscsidriver.yaml
```
* If your k8s version < 1.14
```bash
kubectl create -f deploy/cosfs/kubernetes/cosattacher.yaml
```

Deploys stateful sets for external-attacher sidecar containers for COS CSI driver.

**Deploy COS CSI launcher components:**
* If your k8s version >= 1.18
```bash
kubectl create -f deploy/cosfs/kubernetes/coslauncher-new.yaml
```

* If your k8s version < 1.18
```bash
kubectl create -f deploy/cosfs/kubernetes/coslauncher-old.yaml
```

**Deploy COS CSI driver and related RBACs:**
* If your k8s version >= 1.18
```bash
kubectl create -f deploy/cosfs/kubernetes/rbac.yaml
kubectl create -f deploy/cosfs/kubernetes/cosplugin-new.yaml
```

* If your k8s version >= 1.14 && < 1.18
```bash
kubectl create -f deploy/cosfs/kubernetes/rbac.yaml
kubectl create -f deploy/cosfs/kubernetes/cosplugin-mid.yaml
```

* If your k8s version < 1.14
```bash
kubectl create -f deploy/cosfs/kubernetes/rbac.yaml
kubectl create -f deploy/cosfs/kubernetes/cosplugin-old.yaml
```

Deploys a daemon set with two containers: CSI driver-registrar and the COS CSI driver.

## Verifying the deployment in Kubernetes

After successfully completing the steps above, you should see output similar to this:

```bash
$ kubectl get po -n kube-system
NAME                              READY     STATUS    RESTARTS   AGE
csi-cosplugin-external-runner-0   2/2       Running   0          1h
csi-cosplugin-z9vrj               2/2       Running   0          1h
```

You can try deploying a demo pod from [deploy/cosfs/examples/](/deploy/cosfs/examples) to test the deployment further.

## Test COS plugins with Kubernetes 1.10/1.12

All yaml files referenced in this doc can be found under [deploy/cosfs/examples/](/deploy/cosfs/examples)

## Requirement

secrets are required for mount COS bucket:
https://cloud.tencent.com/document/product/436/6883

First you can create a secret use this command:
```
kubectl create secret generic cos-secret -n kube-system  --from-literal=SecretId=AKIDjustfortest --from-literal=SecretKey=justfortest
```

Then you can find this encoded secret, like this:

```yaml
apiVersion: v1
kind: Secret
type: Opaque
metadata:
  # Replaced by your secret name.
  name: cos-secret
  # Replaced by your secret namespace.
  namespace: kube-system
data:
  # Replaced by your temporary secret file content. You can generate a temporary secret key with these docs:
  # Note: The value must be encoded by base64.
  SecretId: VWVEJxRk5Fb0JGbDA4M...(base64 encode)
  SecretKey: Qa3p4ZTVCMFlQek...(base64 encode)
```

## Create PV and PVC

Currently COS CSI Driver only supports static provisioning, so you need to create a PV first:

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: "pv-cos"
spec:
  accessModes:
  - ReadWriteMany
  capacity:
    storage: 1Gi
  csi:
    driver: com.tencent.cloud.csi.cosfs
    # Specify a unique volumeHandle like bucket name.(this value must different from other pv's volumeHandle)
    volumeHandle: xxx
    volumeAttributes:
      # Replaced by the url of your region.
      url: "http://cos.ap-guangzhou.myqcloud.com"
      # Replaced by the bucket name you want to use.
      bucket: "testbucket-1010101010"
      # You can specify sub-directory of bucket in cosfs command in here.
      # path: "/my-dir"
      # You can specify any other options used by the cosfs command in here.
      #additional_args: "-oallow_other"
    nodePublishSecretRef:
      # Replaced by the name and namespace of your secret.
      name: cos-secret
      namespace: kube-system
```

In general, you only need to sepcify the TencentCloud COS service URL and the bucket name you want to use inside `volumeAttributes`.
If you want to specify other options for the `cosfs` command, you can store them in the `additional_args`.

Then you should see output similar to this:

```bash
$ kubectl get pv
NAME      CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS      CLAIM     STORAGECLASS   REASON    AGE
pv-cos    1Gi        RWX            Retain           Available                                      5s
```

Now you can create a PVC to use the PV above:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
    name: pvc-cos
spec:
  accessModes:
  - ReadWriteMany
  resources:
    requests:
      storage: 1Gi
  # You can specify the pv name manually or just let kubernetes to bind the pv and pvc.
  # volumeName: pv-cos
  # Currently cos only supports static provisioning, the StorageClass name should be empty.
  storageClassName: ""
```

Then you should see that the PVC is already bound to the PV:

```bash
$ kubectl get pvc
NAME      STATUS    VOLUME    CAPACITY   ACCESS MODES   STORAGECLASS   AGE
pvc-cos   Bound     pv-cos    1Gi        RWX                           2s
```

## Create a Pod to use the PVC

You can create a Pod to test the PVC:

```yaml
apiVersion: v1
kind: Pod
metadata:
    name: pod-cos
spec:
  containers:
  - name: pod-cos
    command: ["tail", "-f", "/etc/hosts"]
    image: "centos:latest"
    volumeMounts:
    - mountPath: /data
      name: cos
    resources:
      requests:
        memory: "128Mi"
        cpu: "0.1"
  volumes:
  - name: cos
    persistentVolumeClaim:
      # Replaced by your pvc name.
      claimName: pvc-cos
```

The Pod should work properly:

```bash
$ kubectl get po
NAME      READY     STATUS    RESTARTS   AGE
pod-cos   1/1       Running   0          1m
```

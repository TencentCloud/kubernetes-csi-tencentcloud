# chdfs

This plugin is able to mount TencentCloud CHDFS filesystem to workloads. It only supports static provisioning now.

* **Static Provisioning** - firstly, create a CHDFS on tencent cloud manually; then mount it inside container.

## Building

Build chdfs image:

```sh
docker build -t ${imageName} -f build/chdfs/Dockerfile .
```

## Deploy with Kubernetes

**Requires Kubernetes 1.14+**

**Deploy CHDFS:**

**If your k8s version >= 1.20**
```sh
kubectl apply -f  deploy/chdfs/kubernetes/csidriver-new.yaml
kubectl apply -f  deploy/chdfs/kubernetes/csi-node-rbac.yaml
kubectl apply -f  deploy/chdfs/kubernetes/csi-node.yaml
```

**If your k8s version < 1.20**
```sh
kubectl apply -f  deploy/chdfs/kubernetes/csidriver-old.yaml
kubectl apply -f  deploy/chdfs/kubernetes/csi-node-rbac.yaml
kubectl apply -f  deploy/chdfs/kubernetes/csi-node.yaml
```

## Verifying

After successfully completing the steps above, you should see output similar to this:

```sh
$ kubectl get po -n kube-system | grep chdfs
csi-chdfs-node-fcwd4                 2/2     Running   0          23m
```

## Create PV and PVC

Currently CHDFS CSI Driver only supports static provisioning, so you need to create a PV first:

```sh
kubectl apply -f deploy/chdfs/examples/pv.yaml
```

You only need to sepcify the field `url`, which you can find in chdfs's mount point.

Then you should see output similar to this:

```sh
$ kubectl get pv
NAME           CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS     CLAIM   STORAGECLASS   REASON   AGE
csi-chdfs-pv   10Gi       RWX            Retain           Available                                  37m
```

Now you can create a PVC to use the PV above:

```sh
kubectl apply -f deploy/chdfs/examples/pvc.yaml
```

Then you would see that the PVC bound to the PV:

```sh
$ kubectl get pvc
NAME            STATUS   VOLUME         CAPACITY   ACCESS MODES   STORAGECLASS   AGE
csi-chdfs-pvc   Bound    csi-chdfs-pv   10Gi       RWX                           39m
```

## Create a Pod to use the PVC

You can create a Pod to use the PVC:

```sh
kubectl apply -f deploy/chdfs/examples/pod.yaml
```

The Pod should work properly:

```sh
$ kubectl get pod
NAME                             READY   STATUS    RESTARTS   AGE
csi-chdfs-pod-6bdcf45f89-lrw82   1/1     Running   0          9s
```

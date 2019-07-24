# kubernetes-csi-tencentcloud

`kubernetes-csi-tencentloud` plugins implement interface of [CSI](https://github.com/container-storage-interface/spec)。It can enable your Container Orchestrator use Tencent Cloud storage。

## Install Kubernetes 

**Requirements:**

* Kubernetes v1.13.x+
* kube-apiserver and kubelet need `--allow-privileged=true`
* kubelet configuration：`--feature-gates=VolumeSnapshotDataSource=true,CSINodeInfo=true,CSIDriverRegistry=true,KubeletPluginsWatcher=true`
* apiserver/controller-manager configuration：:  `--feature-gates=VolumeSnapshotDataSource=true,CSINodeInfo=true,CSIDriverRegistry=true`
* scheduler configuration：: `--feature-gates=VolumeSnapshotDataSource=true,CSINodeInfo=true,CSIDriverRegistry=true,VolumeScheduling=true`

#### 1. tencentcloud yunapi secret: 

```
# deploy/kubernetes/secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: csi-tencentcloud
data:
  # value need base64 encoding
  # echo -n "<SECRET_ID>" | base64
  TENCENTCLOUD_CBS_API_SECRET_ID: "<SECRET_ID>"
  TENCENTCLOUD_CBS_API_SECRET_KEY: "<SECRET_KEY>"
```

#### 2. crd:
```
kubectl create -f  deploy/kubernetes/csinodeinfo.yaml
kubectl create -f  deploy/kubernetes/csidriver.yaml
```

#### 3. rbac

```
kubectl apply -f  deploy/kubernetes/csi-attacher-rbac.yaml
kubectl apply -f  deploy/kubernetes/csi-nodeplugin-rbac.yaml
kubectl apply -f  deploy/kubernetes/csi-provisioner-rbac.yaml

```

#### 4. controller,node和plugin

```
kubectl apply -f  deploy/kubernetes/csi-cbsplugin.yaml
kubectl apply -f  deploy/kubernetes/csi-cbsplugin-provisioner.yaml
kubectl apply -f  deploy/kubernetes/csi-cbsplugin-attacher.yaml

```

#### 5. testing

```
storageclass:
    kubectl apply -f  deploy/examples/storageclass-basic.yaml
pvc:
    kubectl apply -f  deploy/examples/pvc.yaml
pod:
    kubectl apply -f  deploy/examples/app.yaml
snapshotclass:
    kubectl apply -f  deploy/examples/snapshoter/snapshoterclass.yaml
snapshot:
    kubectl apply -f  deploy/examples/snapshoter/snapshot.yaml
restore:
    kubectl apply -f  deploy/examples/snapshoter/restore.yaml

```


## StorageClass parameters

**Note**：[examples](https://github.com/TencentCloud/kubernetes-csi-tencentcloud/blob/master/deploy/examples/storageclass-examples.yaml)

* If there are multiple zones of node in your cluster, you can enable topology-aware scheduling of cbs storage volumes with adding `volumeBindingMode: WaitForFirstConsumer` in storageclass, deploy/examples/storageclass-topology.yaml, because cbs volumes can't attach a node with different zone.
* diskType: cbs volume type, `CLOUD_BASIC`,`CLOUD_PREMIUM`,`CLOUD_SSD`.
* diskChargeType: `PREPAID`(need extra parameter), `POSTPAID_BY_HOUR`
* diskChargeTypePrepaidPeriod：`1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 24, 36`
* diskChargePrepaidRenewFlag: If diskType is `PREPAID`, `NOTIFY_AND_AUTO_RENEW`, `NOTIFY_AND_MANUAL_RENEW`, `DISABLE_NOTIFY_AND_MANUAL_RENEW`.
* encrypt: if need encrypt in cbs, `ENCRYPT` is only one value.

## cbs volume size limit, need pvc or pv

* `CLOUD_BASIC`: 10GB-16000GB
* `CLOUD_PREMIUM`: 10GB-16000GB
* `CLOUD_SSD`: 100G-16000GB


## Contributing
If you have any issues or would like to contribute, feel free to open an issue/PR.
# kubernetes-csi-tencentcloud

`kubernetes-csi-tencentloud` plugins implement interface of [CSI](https://github.com/container-storage-interface/spec). It can enable your Container Orchestrator use Tencent [Cloud Block Storage](https://cloud.tencent.com/product/cbs).

## Features
* **Static Provisioning** - firstly, have a CBS disk; then, create PV from the CBS disk and consume the PV from container using PVC.
* **Dynamic Provisioning** - use PVC to request the Kuberenetes to create the CBS disk on behalf of user and consumes the disk from inside container
    * specify zone - which zone the CBS disk will be provisioned in.
        * `allowedTopologies` - The topology key should be `topology.com.tencent.cloud.csi.cbs/zone`.
        * `diskZone` in `StorageClass.parameters` - the zone in `diskZone` is prefered. Then, zone in `allowedTopologies`.
    * **Topology-Aware** - create disk until pod has schedulered, and create disk in the zone which node in. the zone in `diskZone` is prefered
* **Volume Snapshot**
* **Volume Resizing** - expand volume size
* **Volume Attach Limit** - the maximum number of CBS disks that can be attached to one node.(20 CBS disks per node)

## Install Kubernetes
**Note**:
We need know some notes before **Requirements**:
- If setting some feature gates explicitly, we will get some errors. We can se them implicitly start from the beta versions of these feature gates.(e.g. KubeletPluginsWatcher can be not set to kubelet start from 1.12.). Please reference follow table:

| Feature                    | Default    | Stage   | Since   | Until   |
| -------------------------- | ------ | ---- | ---- | ---- |
| `VolumeSnapshotDataSource` | `true` | Beta | 1.17 | -    |
| `CSINodeInfo`              | `true` | Beta | 1.14 | 1.16 |
| `CSIDriverRegistry`        | `true` | Beta | 1.14 | 1.17 |
| `KubeletPluginsWatcher`    | `true` | Beta | 1.12 | 1.12 |
| `VolumeScheduling`         | `true` | Beta | 1.10 | 1.12 |
| `ExpandCSIVolumes`         | `true` | Beta | 1.16 | - |

**Requirements:**

* Kubernetes v1.14.x+
* kube-apiserver and kubelet need `--allow-privileged=true` (for v1.15.x+, kubelet defaults to set `--allow-privileged` to true. if still set it explicitly, will get error.)
* kubelet configuration：`--feature-gates=VolumeSnapshotDataSource=true`
* apiserver/controller-manager configuration：:  `--feature-gates=VolumeSnapshotDataSource=true`
* scheduler configuration：: `--feature-gates=VolumeSnapshotDataSource=true,VolumeScheduling=true`

### tencentcloud yunapi secret
***Note: If in TKE cluster, this step is optional; if not, must create this secret.***

```yaml
# deploy/kubernetes/secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: cbs-csi-api-key
  namespace: kube-system
data:
  # value need base64 encoding
  # echo -n "<SECRET_ID>" | base64
  TENCENTCLOUD_CBS_API_SECRET_ID: "<SECRET_ID>"
  TENCENTCLOUD_CBS_API_SECRET_KEY: "<SECRET_KEY>"
```

### rbac

```yaml
kubectl apply -f  deploy/cbs/kubernetes/csi-controller-rbac.yaml
kubectl apply -f  deploy/cbs/kubernetes/csi-node-rbac.yaml
```

### controller,node plugin

```yaml
kubectl apply -f  deploy/cbs/kubernetes/csi-controller.yaml
kubectl apply -f  deploy/cbs/kubernetes/csi-node.yaml
kubectl apply -f  deploy/cbs/kubernetes/snapshot-crd.yaml
```

### examples

```yaml
storageclass:
    kubectl apply -f  deploy/cbs/examples/storageclass-basic.yaml
pvc:
    kubectl apply -f  deploy/cbs/examples/pvc.yaml
pod:
    kubectl apply -f  deploy/cbs/examples/app.yaml
snapshotclass:
    kubectl apply -f  deploy/cbs/examples/snapshoter/snapshoterclass.yaml
snapshot:
    kubectl apply -f  deploy/cbs/examples/snapshoter/snapshot.yaml
restore:
    kubectl apply -f  deploy/cbs/examples/snapshoter/restore.yaml
```

## StorageClass parameters

**Note**：[examples](https://github.com/TencentCloud/kubernetes-csi-tencentcloud/blob/master/deploy/cbs/examples/storageclass-examples.yaml)

* If there are multiple zones of node in your cluster, you can enable topology-aware scheduling of cbs storage volumes with adding `volumeBindingMode: WaitForFirstConsumer` in storageclass, deploy/examples/storageclass-topology.yaml, because cbs volumes can't attach a node with different zone.
* diskType: cbs volume type, `CLOUD_PREMIUM`,`CLOUD_SSD`,`CLOUD_HSSD`.
* diskChargeType: `PREPAID`(need extra parameter), `POSTPAID_BY_HOUR`
* diskChargeTypePrepaidPeriod：`1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 24, 36`
* diskChargePrepaidRenewFlag: If diskType is `PREPAID`, `NOTIFY_AND_AUTO_RENEW`, `NOTIFY_AND_MANUAL_RENEW`, `DISABLE_NOTIFY_AND_MANUAL_RENEW`.
* encrypt: if need encrypt in cbs, `ENCRYPT` is only one value.
* disktags: add tags to cbs volume. e.g. `a:b,c:d`
* throughputperformance: if need extra performance for hssd/tssd. e.g. `100`. https://cloud.tencent.com/document/product/362/51896
* cdcid: `CdcId`

## cbs volume size limit, need pvc or pv

* `CLOUD_PREMIUM`: 10GB-32000GB
* `CLOUD_SSD`: 20G-32000GB
* `CLOUD_HSSD`: 20G-32000GB
# kubernetes-csi-tencentcloud

`kubernetes-csi-tencentloud` 是腾讯云 `Cloud Block Storage` 服务的一个满足 [CSI](https://github.com/container-storage-interface/spec) 标准实现的插件。这个插件可以让你在 Kubernetes 上使用 Cloud Block Storage。

## 特性
* **Static Provisioning** - 为一个已有的CBS盘创建PV，并在容器中使用PVC来使用它
* **Dynamic Provisioning** - 在容器需要使用时，根据PVC去创建CBS盘
    * specify zone - 指定要在哪个zone创建CBS盘
        * `allowedTopologies` - topology key是`topology.com.tencent.cloud.csi.cbs/zone`.
        * `diskZone` in `StorageClass.parameters` - `diskZone`中配置的zone优先级最高. 之后才是`allowedTopologies`中的zone
    * **Topology-Aware** - pod被调度完以后，在相应node所在zone创建CBS盘. 如果同时`diskZone`已配置，优先`diskZone`
* **Volume Snapshot** - 磁盘快照
* **Volume Resizing** - 磁盘扩容
* **Volume Attach Limit** - 单节点最大能attach的CBS盘数量.(每个节点最大可attach 20块CBS盘)

## 在 Kubernetes 上安装

**注意**:
在讲述**前置要求**之前，对于各组件设置参数启动项有些要注意的地方：
- 有些feature gates在GA以后的版本不能再被显式设置，否则可能导致报错。实际上这些feature gates在beta版本开始则无需添加。下表整理了涉及到feature gates的beta版本的表格，在给kubelet、master/controllermanager、scheduler设置启动参数时，可以基于此来做取舍.（举例：KubeletPluginsWatcher在1.12及以上版本则无须添加）

| 特性                         | 默认值    | 阶段   | 起始   | 直到   |
| -------------------------- | ------ | ---- | ---- | ---- |
| `VolumeSnapshotDataSource` | `true` | Beta | 1.17 | -    |
| `CSINodeInfo`              | `true` | Beta | 1.14 | 1.16 |
| `CSIDriverRegistry`        | `true` | Beta | 1.14 | 1.17 |
| `KubeletPluginsWatcher`    | `true` | Beta | 1.12 | 1.12 |
| `VolumeScheduling`         | `true` | Beta | 1.10 | 1.12 |
| `ExpandCSIVolumes`         | `true` | Beta | 1.16 | - |

**前置要求:**

* Kubernetes v1.13.x及以上
* kube-apiserver 和 kubelet 的 `--allow-privileged` flag 都要设置为 true (针对 v1.15.x 及以上版本, kubelet 默认设置 `--allow-privileged` 为 true，如果仍然显式设置，则会报错 )
* 所有节点的kubelet 需要添加的启动项为：--feature-gates=VolumeSnapshotDataSource=true,CSINodeInfo=true,CSIDriverRegistry=true,KubeletPluginsWatcher=true
* apiserver/controller-manager:  --feature-gates=VolumeSnapshotDataSource=true,CSINodeInfo=true,CSIDriverRegistry=true
* scheduler: --feature-gates=VolumeSnapshotDataSource=true,CSINodeInfo=true,CSIDriverRegistry=true,VolumeScheduling=true



####  使用腾讯云 API Credential 创建 kubernetes secret:
***注： 如果是自建集群，必须创建；而如果是TKE集群环境，可以不创建该secret，driver中默认会根据TKE_QCSRole获取临时秘钥。***

```
#  参考示例 deploy/kubernetes/secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: csi-tencentcloud
  namespace: kube-system
data:
  # 需要注意的是,secret 的 value 需要进行 base64 编码
  #   echo -n "<SECRET_ID>" | base64
  TENCENTCLOUD_CBS_API_SECRET_ID: "<SECRET_ID>"
  TENCENTCLOUD_CBS_API_SECRET_KEY: "<SECRET_KEY>"
```


#### 创建rbac

创建attacher,provisioner,plugin需要的rbac：

```
kubectl apply -f  deploy/cbs/kubernetes/csi-controller-rbac.yaml
kubectl apply -f  deploy/cbs/kubernetes/csi-node-rbac.yaml
```

#### 创建controller,node和plugin
创建controller plugin和node plugin

```
kubectl apply -f  deploy/cbs/kubernetes/csi-controller.yaml
kubectl apply -f  deploy/cbs/kubernetes/csi-node.yaml
```

#### 简单测试验证

```
创建storageclass:
    kubectl apply -f  deploy/examples/storageclass-basic.yaml
创建pvc:
    kubectl apply -f  deploy/examples/pvc.yaml
创建申请pvc的pod:
    kubectl apply -f  deploy/examples/app.yaml
```


## StorageClass 支持的参数

**Note**：可以参考[示例](https://github.com/TencentCloud/kubernetes-csi-tencentcloud/blob/master/deploy/cbs/examples/storageclass-examples.yaml)

* 如果您集群中的节点存在多个可用区，那么您可以开启cbs存储卷的拓扑感知调度，需要在storageclass中添加`volumeBindingMode: WaitForFirstConsumer`，如deploy/examples/storageclass-topology.yaml，否则可能会出现cbs存储卷因跨可用区而挂载失败。
* diskType: 代表要创建的 cbs 盘的类型；值为 `CLOUD_BASIC` 代表创建普通云盘，值为 `CLOUD_PREMIUM` 代表创建高性能云盘，值为 `CLOUD_SSD` 代表创建 ssd 云盘
* diskChargeType: 代表云盘的付费类型；值为 `PREPAID` 代表预付费，值为 `POSTPAID_BY_HOUR` 代表按量付费，需要注意的是，当值为 `PREPAID` 的时候需要指定额外的参数
* diskChargeTypePrepaidPeriod：代表购买云盘的时长，当付费类型为 `PREPAID` 时需要指定，可选的值包括 `1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 24, 36`，单位为月
* diskChargePrepaidRenewFlag: 代表云盘的自动续费策略，当付费类型为 `PREPAID` 时需要指定，值为`NOTIFY_AND_AUTO_RENEW` 代表通知过期且自动续费，值为 `NOTIFY_AND_MANUAL_RENEW` 代表通知过期不自动续费，值为 `DISABLE_NOTIFY_AND_MANUAL_RENEW` 代表不通知过期不自动续费
* encrypt: 代表云盘是否加密，当指定此参数时，唯一可选的值为 `ENCRYPT`

## 不同类型云盘的大小限制

* 普通云硬盘提供最小 100 GB 到最大 16000 GB 的规格选择，支持 40-100MB/s 的 IO 吞吐性能和 数百-1000 的随机 IOPS 性能。
* 高性能云硬盘提供最小 50 GB 到最大 16000 GB 的规格选择。
* SSD 云硬盘提供最小 100 GB 到最大 16000 GB 的规格选择，单块 SSD 云硬盘最高可提供 24000 随机读写IOPS、260MB/s吞吐量的存储性能。


## 反馈和建议
如果你在使用过程中遇到任何问题或者有任何建议，欢迎通过 Issue 反馈。

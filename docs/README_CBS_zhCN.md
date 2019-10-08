# kubernetes-csi-tencentcloud

`kubernetes-csi-tencentloud` 是腾讯云 `Cloud Block Storage` 服务的一个满足 [CSI](https://github.com/container-storage-interface/spec) 标准实现的插件。这个插件可以让你在 Kubernetes 上使用 Cloud Block Storage。

## 在 Kubernetes 上安装

**前置要求:**

* Kubernetes v1.13.x及以上
* kube-apiserver 和 kubelet 的 `--allow-privileged` flag 都要设置为 true
* 所有节点的kubelet 需要添加的启动项为：--feature-gates=VolumeSnapshotDataSource=true,CSINodeInfo=true,CSIDriverRegistry=true,KubeletPluginsWatcher=true
* apiserver/controller-manager:  --feature-gates=VolumeSnapshotDataSource=true,CSINodeInfo=true,CSIDriverRegistry=true
* scheduler: --feature-gates=VolumeSnapshotDataSource=true,CSINodeInfo=true,CSIDriverRegistry=true,VolumeScheduling=true

#### 1. 使用腾讯云 API Credential 创建 kubernetes secret: 

```
#  参考示例 deploy/kubernetes/secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: csi-tencentcloud
data:
  # 需要注意的是,secret 的 value 需要进行 base64 编码
  #   echo -n "<SECRET_ID>" | base64
  TENCENTCLOUD_CBS_API_SECRET_ID: "<SECRET_ID>"
  TENCENTCLOUD_CBS_API_SECRET_KEY: "<SECRET_KEY>"
```

#### 2. 部署 CSI1.0 需要的crd:
```
kubectl create -f  deploy/kubernetes/csinodeinfo.yaml
kubectl create -f  deploy/kubernetes/csidriver.yaml
```

#### 3. 部署rbac

创建attacher,provisioner,plugin需要的rbac：

```
kubectl apply -f  deploy/kubernetes/csi-attacher-rbac.yaml
kubectl apply -f  deploy/kubernetes/csi-nodeplugin-rbac.yaml
kubectl apply -f  deploy/kubernetes/csi-provisioner-rbac.yaml

```

#### 4. 创建controller,node和plugin
创建pluginserver的daemonset, controller和node的statefulset

```
kubectl apply -f  deploy/kubernetes/csi-cbsplugin.yaml
kubectl apply -f  deploy/kubernetes/csi-cbsplugin-provisioner.yaml
kubectl apply -f  deploy/kubernetes/csi-cbsplugin-attacher.yaml

```

#### 5.简单测试验证

```
创建storageclass:
    kubectl apply -f  deploy/examples/storageclass-basic.yaml
创建pvc:
    kubectl apply -f  deploy/examples/pvc.yaml
创建申请pvc的pod:
    kubectl apply -f  deploy/examples/app.yaml
```


## StorageClass 支持的参数

**Note**：可以参考[示例](https://github.com/TencentCloud/kubernetes-csi-tencentcloud/blob/master/deploy/examples/storageclass-examples.yaml)

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

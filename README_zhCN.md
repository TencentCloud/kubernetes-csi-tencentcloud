# kubernetes-csi-tencentcloud

`kubernetes-csi-tencentloud` 是腾讯云 `Cloud Block Storage` 服务的一个满足 [CSI](https://github.com/container-storage-interface/spec) 标准实现的插件。这个插件可以让你在 Kubernetes 上使用 Cloud Block Storage。

## 编译

### 编译二进制文件
将此项目 clone 到 GOPATH 下，假设 GOPATH 为 /root/go

```
mkdir -p /root/go/src/github.com/tencentcloud/
git clone https://github.com/tencentcloud/kubernetes-csi-tencentloud.git /root/go/src/github.com/tencentcloud/kubernetes-csi-tencentloud
cd /root/go/src/github.com/tencentcloud/kubernetes-csi-tencentloud
go build -v
```

### 打包 Docker Image (需要 Docker 17.05 或者更高版本)

```
docker build -f Dockerfile.multistage.cbs -t kubernetes-csi-tencentloud:latest .
```

## 在 Kubernetes 上安装

**前置要求:**

* Kubernetes v1.10.x
* kube-apiserver 和 kubelet 的 `--allow-privileged` flag 都要设置为 true

#### 1. 使用腾讯云 API Credential 创建 kubernetes secret: 

```
# deploy/examples/secret.yaml
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

#### 2. 部署 CSI 插件和 sidecar 容器:
```
kubectl create -f https://raw.githubusercontent.com/TencentCloud/kubernetes-csi-tencentcloud/master/deploy/kubernetes/deploy.yaml
```

这条命令会做的事情：

* 创建名为 `csi-tencentcloud` 的 cluster role 和 service account 以及对应的 cluster role binding
* 在集群内以 statefulset 的形式创建 provisioner 和 attacher
* 在集群内以 daemonset 的形式创建 mounter
* 创建名为 `cbs-csi` 的 storageclass

#### 3. 测试和验证

创建 kubernetes persistence volume claim：

```
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: csi-pvc
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 20Gi
  storageClassName: cbs-csi
```

创建使用 pvc 的 pod

```
kind: Pod
apiVersion: v1
metadata:
  name: csi-app
spec:
  containers:
    - name: csi
      image: busybox
      volumeMounts:
      - mountPath: "/data"
        name: csi-volume
      command: [ "sleep", "1000000" ]
  volumes:
    - name: csi-volume
      persistentVolumeClaim:
        claimName: csi-pvc
```

## StorageClass 支持的参数

**Note**：可以参考[示例](https://github.com/TencentCloud/kubernetes-csi-tencentcloud/blob/master/deploy/examples/storageclass-examples.yaml)


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

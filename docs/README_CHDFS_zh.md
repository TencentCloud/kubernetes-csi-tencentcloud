# chdfs

chdfs 插件能够将腾讯云 chdfs 文件系统挂载到对应工作负载中，目前只支持 **Static Provisioning** 模式。

* **Static Provisioning**：为一个已有的 chdfs 文件系统创建 pv，并创建对应 pvc 来使用它。

## 插件镜像

通过执行以下命令来构建 chdfs 插件镜像:

```sh
docker build -t ${imageName} -f build/chdfs/Dockerfile .
```

## 插件部署

**需要集群 Kubernetes 版本大于等于 1.14**

通过执行以下命令来部署 chdfs 插件:

**若集群 k8s 版本 >= 1.20**
```sh
kubectl apply -f  deploy/chdfs/kubernetes/csidriver-new.yaml
kubectl apply -f  deploy/chdfs/kubernetes/csi-node-rbac.yaml
kubectl apply -f  deploy/chdfs/kubernetes/csi-node.yaml
```

**若集群 k8s 版本 < 1.20**
```sh
kubectl apply -f  deploy/chdfs/kubernetes/csidriver-old.yaml
kubectl apply -f  deploy/chdfs/kubernetes/csi-node-rbac.yaml
kubectl apply -f  deploy/chdfs/kubernetes/csi-node.yaml
```

通过执行以下命令来查看插件是否处于 Running 状态:

```sh
$ kubectl get po -n kube-system | grep chdfs
csi-chdfs-node-fcwd4                 2/2     Running   0          23m
```

## 插件使用

首先需要在[腾讯云 chdfs 控制台]( https://console.cloud.tencent.com/chdfs/filesystem )完成[文件系统]( https://cloud.tencent.com/document/product/1105/37234 )、[权限组]( https://cloud.tencent.com/document/product/1105/37235 )，[权限规则]( https://cloud.tencent.com/document/product/1105/37236 )和[挂载点]( https://cloud.tencent.com/document/product/1105/37237 )的创建。

注意：
- 创建文件系统时，在超级用户中添加 root。
- 创建权限组时，所选 vpc 应与创建集群时一致。
- 创建权限规则时，确保集群各节点都能实现对规则的匹配。

### 创建 pv

通过执行以下命令来创建 pv:

```sh
# 在 pv 所支持参数中，只有 url 是必须配置的。
kubectl apply -f deploy/chdfs/examples/pv.yaml
```

参数说明：
- **allowother**: 是否允许其他用户访问。
- **sync**: 是否将内存的任何修改都实时同步到 chdfs 文件系统中。
- **debug**: 是否在日志中显示详细的 fuse 接口调用。
- **url**: 创建 chdfs 文件系统挂载点后自动生成，可在控制台相应页面查看。
- **additional_args**: chdfs 支持自定义挂载参数，各参数间以空格隔开，例如：`client.renew-session-lease-time-sec=100 cache.read.read-ahead-block-num=100`。支持参数及说明如下：

|名称|默认值|描述|
|-|-|-|
|security.ssl-ca-path|-|CA路径，例：/etc/ssl/certs/ca-bundle.crt|
|client.renew-session-lease-time-sec|10|会话续租时间（s）|
|client.mount-sub-dir|根目录|挂载子目录|
|client.user|当前用户名|用户名|
|client.group|当前组名|组名|
|client.force-sync|false|强制sync开关，不依赖“-o sync”|
|cache.update-sts-time-sec|30|数据读写临时密钥刷新时间（s）|
|cache.cos-client-timeout-sec|5|数据上传/下载超时时间（s）|
|cache.inode-attr-expired-time-sec|30|inode属性缓存有效时间（s）|
|cache.read.block-expired-time-sec|10|【读操作】单Fd数据读缓存有效时间（s）（block粒度）|
|cache.read.max-block-num|256|【读操作】单Fd数据读缓存block最大数量|
|cache.read.read-ahead-block-num|15|【读操作】单Fd预读block数量（read-ahead-block-num < max-block-num）|
|cache.read.max-cos-load-qps|1024|【读操作】多Fd数据下载最大QPS（QPS * 1MB < 网卡带宽）|
|cache.read.load-thread-num|128|【读操作】多Fd数据下载worker数量|
|cache.read.select-thread-num|64|【读操作】多Fd元数据查询worker数量|
|cache.read.rand-read|false|【读操作】随机读场景开关|
|cache.write.max-mem-table-range-num|32|【写操作】单Fd当前数据写缓存range最大数量|
|cache.write.max-mem-table-size-mb|64|【写操作】单Fd当前数据写缓存最大容量（MB）|
|cache.write.max-cos-flush-qps|256|【写操作】多Fd数据上传最大QPS（QPS * 4MB < 网卡带宽）|
|cache.write.flush-thread-num|128|【写操作】多Fd数据上传worker数量|
|cache.write.commit-queue-len|100|【写操作】单Fd元数据提交队列长度|
|cache.write.max-commit-heap-size|500|【写操作】单Fd元数据提交最大容量（无需设置）|
|cache.write.auto-merge|true|【写操作】单Fd写时自动合并文件碎片开关|
|cache.write.auto-sync|false|【写操作】单Fd写时自动刷脏页开关|
|cache.write.auto-sync-time-ms|1000|【写操作】单Fd写时自动刷脏页时间周期（ms）|
|log.level|info|日志级别|
|log.file.filename|default.log|日志文件名|
|log.file.log-rotate|true|日志分割|
|log.file.max-size|-|单个日志文件最大容量（MB）|
|log.file.max-days|-|单个日志文件保存最长时间（天）|
|log.file.max-backups|-|历史日志文件最多文件数量|

### 创建 pvc

通过执行以下命令来创建 pvc:

```sh
kubectl apply -f deploy/chdfs/examples/pvc.yaml
```

通过执行以下命令来查看 pvc 与 pv 的绑定状态:

```sh
$ kubectl get pvc csi-chdfs-pvc
NAME            STATUS   VOLUME         CAPACITY   ACCESS MODES   STORAGECLASS   AGE
csi-chdfs-pvc   Bound    csi-chdfs-pv   10Gi       RWX                           39m
```

### 创建 pod

通过执行以下命令来创建 pod:

```sh
kubectl apply -f deploy/chdfs/examples/pod.yaml
```

通过执行以下命令来查看 pod 是否处于 Running 状态:

```sh
$ kubectl get pod
NAME                             READY   STATUS    RESTARTS   AGE
csi-chdfs-pod-6bdcf45f89-lrw82   1/1     Running   0          9s
```

### 多目录挂载说明

需要预先在对应的chdfs文件系统中创建对应的子目录,然后创建pvc,pv来指定到对应的子目录,以下是在pv yaml中指定子目录的示例

```yaml
  csi:
    driver: com.tencent.cloud.csi.chdfs
    volumeAttributes:
      additional_args: client.mount-sub-dir=/aaa    #指定子目录
      allowother: "true"
      debug: "true"
      sync: "false"
      url: xxx.chdfs.ap-guangzhou.myqcloud.com
    volumeHandle: csi-chdfs-pv-1
```

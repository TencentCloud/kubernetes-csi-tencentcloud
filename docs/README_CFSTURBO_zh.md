# cfsturbo

cfsturbo 插件能够将腾讯云 cfsturbo 文件系统挂载到对应工作负载中，支持 **Dynamic Provisioning** 和 **Static Provisioning** 两种模式。

* **Dynamic Provisioning**：为一个已有的 cfsturbo 文件系统创建 sc，再创建 pvc 指定该 sc，后由插件动态创建 pv 并在文件系统中创建对应子目录。
* **Static Provisioning**：为一个已有的 cfsturbo 文件系统创建 pv，并创建对应 pvc 来使用它。

## 插件镜像

通过执行以下命令来构建并上传 cfsturbo 插件镜像:

```sh
make cfsturbo
```

## 插件部署

**需要集群 Kubernetes 版本大于等于 1.14**

**若集群 k8s 版本 >= 1.20**
```sh
kubectl apply -f  deploy/cfsturbo/kubernetes/csidriver-new.yaml
kubectl apply -f  deploy/cfsturbo/kubernetes/csi-node.yaml
kubectl apply -f  deploy/cfsturbo/kubernetes/csi-node-rbac.yaml
kubectl apply -f  deploy/cfsturbo/kubernetes/csi-controller.yaml
```

**若集群 k8s 版本 < 1.20**
```sh
kubectl apply -f  deploy/cfsturbo/kubernetes/csidriver-old.yaml
kubectl apply -f  deploy/cfsturbo/kubernetes/csi-node.yaml
kubectl apply -f  deploy/cfsturbo/kubernetes/csi-node-rbac.yaml
kubectl apply -f  deploy/cfsturbo/kubernetes/csi-controller.yaml
```

通过执行以下命令来查看插件是否处于 Running 状态:

```sh
$ kubectl get pod -n kube-system | grep cfsturbo
cfsturbo-csi-controller-66f4bf6ddb-m9zj2   2/2     Running   0             10m
cfsturbo-csi-node-9pgxm                    2/2     Running   0             10m
cfsturbo-csi-node-ztc7k                    2/2     Running   0             10m
```

**注意**：  
在部署前需配置 [csi-controller.yaml](../deploy/cfsturbo/kubernetes/csi-controller.yaml) 文件中环境变量 `CLUSTER_ID` 的值为集群 id，该变量与 **Dynamic Provisioning** 模式中动态生成的子目录路径有关。

## 插件使用

首先需要在[腾讯云文件存储控制台]( https://console.cloud.tencent.com/cfs )完成 turbo 类型文件系统的创建。

**注意**：  
使用 cfsturbo 插件挂载 turbo 类型文件系统，需预先使用 [CFS 客户端工具](https://console.cloud.tencent.com/cfs/fs/cvmInitialize) 在集群节点内安装对应客户端。

### Dynamic Provisioning

完成 sc、pvc 创建后，插件会在文件系统中创建如 `/cfs/$CLUSTER_ID/$pvname` 格式的子目录；完成 pod 创建后，插件会将上一步创建的子目录挂载到容器对应目录下；完成 pod、pvc 删除后，插件会根据 sc 中配置的回收策略来选择是否进行子目录清理。

#### 创建 sc

通过执行以下命令来创建 sc:

```sh
kubectl apply -f deploy/cfsturbo/examples/dynamic/sc.yaml
```

参数说明：
- **parameters.fsid**: 文件系统 fsid，即文件系统挂载点信息中的 `ID` 参数。
- **parameters.host**: 文件系统 ip 地址，可在文件系统挂载点信息中查看。
- **reclaimPolicy**：子目录回收策略，若配置为 `Delete` 会在删除 pvc 后对应删除子目录与其中数据；若配置为 `Retain` 则保留子目录与其中数据。

#### 创建 pvc

通过执行以下命令来创建 pvc:

```sh
kubectl apply -f deploy/cfsturbo/examples/dynamic/pvc.yaml
```

通过执行以下命令来查看 pvc 与 pv 的绑定状态:

```sh
$ kubectl get pvc csi-cfsturbo-pvc-dynamic
NAME                       STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS      AGE
csi-cfsturbo-pvc-dynamic   Bound    pvc-5aa37ef6-f845-45c8-997d-7abd2af5a0cc   10Gi       RWX            csi-cfsturbo-sc   27s
```

#### 创建 pod

通过执行以下命令来创建 pod:

```sh
kubectl apply -f deploy/cfsturbo/examples/dynamic/pod.yaml
```

通过执行以下命令来查看 pod 是否处于 Running 状态:

```sh
$ kubectl get pod
NAME                                        READY   STATUS    RESTARTS   AGE
csi-cfsturbo-pod-dynamic-6b5f578dfc-9f697   1/1     Running   0          73s
```

### Static Provisioning

#### 创建 pv

通过执行以下命令来创建 pv:

```sh
kubectl apply -f deploy/cfsturbo/examples/static/pv.yaml
```

参数说明：
- **metadata.name**: 创建 PV 名称。
- **spec.csi.volumeHandle**: 与 PV 名称保持一致。
- **spec.csi.volumeAttributes.host**: 文件系统 ip 地址，可在文件系统挂载点信息中查看。
- **spec.csi.volumeAttributes.fsid**: 文件系统 fsid，即文件系统挂载点信息中的 `ID` 参数。
- **spec.csi.volumeAttributes.rootdir**: 文件系统根目录，不填写默认为 “/cfs”（挂载到 “/cfs” 目录可相对提高整体挂载性能）。如需指定其它目录挂载，须确保该目录在文件系统中存在。
- **spec.csi.volumeAttributes.path**: 文件系统子目录，不填写默认为 “/”。如需指定子目录挂载，须确保该子目录在文件系统 rootdir 中存在。容器最终访问到的是文件系统中 rootdir+path 目录（默认为 “/cfs/” 目录）。

#### 创建 pvc

通过执行以下命令来创建 pvc:

```sh
kubectl apply -f deploy/cfsturbo/examples/static/pvc.yaml
```

通过执行以下命令来查看 pvc 与 pv 的绑定状态:

```sh
$ kubectl get pvc csi-cfsturbo-pvc-static
NAME                      STATUS   VOLUME                   CAPACITY   ACCESS MODES   STORAGECLASS   AGE
csi-cfsturbo-pvc-static   Bound    csi-cfsturbo-pv-static   10Gi       RWX                           42s
```

#### 创建 pod

通过执行以下命令来创建 pod:

```sh
kubectl apply -f deploy/cfsturbo/examples/static/pod.yaml
```

通过执行以下命令来查看 pod 是否处于 Running 状态:

```sh
$ kubectl get pod
NAME                                        READY   STATUS    RESTARTS   AGE
csi-cfsturbo-pod-static-7c4c88d7fc-hqlxf    1/1     Running   0          78s
```

# kubernetes-csi-tencentcloud

A Container Storage Interface ([CSI](https://github.com/container-storage-interface/spec)) Driver for TencentCloud Cloud Block Storage. The CSI plugin allows you to use TencentCloud Cloud Block Storage with Kubernetes.

## Installing to Kubernetes

**Requirements:**

* Kubernetes v1.10.x
* `--allow-privileged` flag must be set to true for both the API server and the kubelet
* (if you use Docker) the Docker daemon of the cluster nodes must allow shared mounts

#### 1. Create a secret with your TencentCloud API Credential: 

```
# deploy/kubernetes/secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: csi-tencentcloud
data:
  # value in secret need to base64 encoded
  #   echo -n "<SECRET_ID>" | base64
  TENCENTCLOUD_CBS_API_SECRET_ID: "<SECRET_ID>"
  TENCENTCLOUD_CBS_API_SECRET_KEY: "<SECRET_KEY>"
```

#### 2. Deploy the CSI plugin and sidecars:

##### Create Kubernetes role and service account for csi containers
```
apiVersion: v1
kind: ServiceAccount
metadata:
  name: csi-tencentcloud
---
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: csi-tencentcloud
rules:
  - apiGroups: [""]
    resources: ["persistentvolumes"]
    verbs: ["get", "list", "watch", "create", "delete", "update"]
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["get", "list", "watch", "create", "update", "patch"]
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list"]
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["volumeattachments"]
    verbs: ["get", "list", "watch", "update"]
  - apiGroups: ["storage.k8s.io"]
    resources: ["storageclasses"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["namespaces"]
    verbs: ["get", "list"]

---
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: csi-tencentcloud
subjects:
  - kind: ServiceAccount
    name: csi-tencentcloud
    namespace: default
roleRef:
  kind: ClusterRole
  name: csi-tencentcloud
  apiGroup: rbac.authorization.k8s.io
```

##### Deploy csi node service

```
# deploy/kubernetes/mounter.yaml
kind: DaemonSet
apiVersion: apps/v1beta2
metadata:
  name: csi-tencentcloud
spec:
  selector:
    matchLabels:
      app: csi-tencentcloud
  template:
    metadata:
      labels:
        app: csi-tencentcloud
    spec:
      serviceAccount: csi-tencentcloud
      hostNetwork: true
      hostIPC: true
      containers:
        - name: driver-registrar
          image: ccr.ccs.tencentyun.com/library/csi-driver-registrar:0.2.0
          args:
            - "--v=5"
            - "--csi-address=$(ADDRESS)"
          env:
            - name: ADDRESS
              value: /csi/csi.sock
            - name: KUBE_NODE_NAME
              valueFrom:
                fieldRef:
                  fieldPath: spec.nodeName
          volumeMounts:
            - name: plugin-dir
              mountPath: /csi/
        - name: csi-tencentcloud
          securityContext:
            privileged: true
            capabilities:
              add: ["SYS_ADMIN"]
            allowPrivilegeEscalation: true
          image: ccr.ccs.tencentyun.com/library/csi-tencentcloud-cbs:latest
          command:
          - "/bin/csi-tencentcloud"
          args:
          - "--v=5"
          - "--logtostderr=true"
          - "--endpoint=unix:///csi/csi.sock"
          env:
            - name: TENCENTCLOUD_CBS_API_SECRET_ID
              valueFrom:
                secretKeyRef:
                  name: csi-tencentcloud
                  key: TENCENTCLOUD_CBS_API_SECRET_ID
            - name: TENCENTCLOUD_CBS_API_SECRET_KEY
              valueFrom:
                secretKeyRef:
                  name: csi-tencentcloud
                  key: TENCENTCLOUD_CBS_API_SECRET_KEY
          imagePullPolicy: "Always"
          volumeMounts:
            - name: plugin-dir
              mountPath: /csi/
            - name: pods-mount-dir
              mountPath: /var/lib/kubelet/pods
              mountPropagation: "Bidirectional"
            - name: global-mount-dir
              mountPath: /var/lib/kubelet/plugins/kubernetes.io/csi
              mountPropagation: "Bidirectional"
            - mountPath: /dev
              name: device-dir
      volumes:
        - name: plugin-dir
          hostPath:
            path: /var/lib/kubelet/plugins/com.tencent.cloud.csi.cbs
            type: DirectoryOrCreate
        - name: pods-mount-dir
          hostPath:
            path: /var/lib/kubelet/pods
            type: Directory
        - name: global-mount-dir
          hostPath:
            path: /var/lib/kubelet/plugins/kubernetes.io/csi
            type: Directory
        - name: device-dir
          hostPath:
            path: /dev
```

##### Deploy csi controller service
```
# deploy/kubernetes/provisionerandattacher.yaml
kind: StatefulSet
apiVersion: apps/v1beta1
metadata:
  name: csi-tencentcloud
spec:
  serviceName: "csi-tencentcloud"
  replicas: 1
  template:
    metadata:
      labels:
        app: csi-tencentcloud
    spec:
      serviceAccount: csi-tencentcloud
      containers:
        - name: csi-provisioner
          image: ccr.ccs.tencentyun.com/library/csi-external-provisioner:0.2.0
          args:
            - "--provisioner=com.tencent.cloud.csi.cbs"
            - "--csi-address=$(ADDRESS)"
            - "--v=5"
            - "-connection-timeout=120s"
          env:
            - name: ADDRESS
              value: /var/lib/csi/sockets/pluginproxy/csi.sock
          imagePullPolicy: "IfNotPresent"
          volumeMounts:
            - name: socket-dir
              mountPath: /var/lib/csi/sockets/pluginproxy/
        - name: csi-attacher
          image: ccr.ccs.tencentyun.com/library/csi-external-attacher:0.2.0
          args:
            - "--v=5"
            - "--csi-address=$(ADDRESS)"
          env:
            - name: ADDRESS
              value: /var/lib/csi/sockets/pluginproxy/csi.sock
          imagePullPolicy: "IfNotPresent"
          volumeMounts:
            - name: socket-dir
              mountPath: /var/lib/csi/sockets/pluginproxy/
        - name: csi-tencentcloud
          image: ccr.ccs.tencentyun.com/library/csi-tencentcloud-cbs:latest
          command:
          - "/bin/csi-tencentcloud"
          args:
          - "--v=5"
          - "--logtostderr=true"
          - "--endpoint=unix:///var/lib/csi/sockets/pluginproxy/csi.sock"
          env:
            - name: TENCENTCLOUD_CBS_API_SECRET_ID
              valueFrom:
                secretKeyRef:
                  name: csi-tencentcloud
                  key: TENCENTCLOUD_CBS_API_SECRET_ID
            - name: TENCENTCLOUD_CBS_API_SECRET_KEY
              valueFrom:
                secretKeyRef:
                  name: csi-tencentcloud
                  key: TENCENTCLOUD_CBS_API_SECRET_KEY
          imagePullPolicy: "Always"
          volumeMounts:
            - name: socket-dir
              mountPath: /var/lib/csi/sockets/pluginproxy/
      volumes:
        - name: socket-dir
          emptyDir: {}

```

##### Create kubernetes storage class

```
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: cbs-csi
provisioner: com.tencent.cloud.csi.cbs
```

#### 3. Test and verify:

Create a PersistentVolumeClaim. 

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
      storage: 10Gi
  storageClassName: cbs-csi
```

After that create a Pod that refers to this volume. When the Pod is created, the volume will be attached, formatted and mounted to the specified Container

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

## Contributing
If you have any issues or would like to contribute, feel free to open an issue/PR
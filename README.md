# kubernetes-csi-tencentcloud

## Important Release Note
User should apply the secret.yaml to cluster, otherwise the csi will fail to provision/attach because of access authority.

`kubernetes-csi-tencentcloud` plugins implement interface of [CSI](https://github.com/container-storage-interface/spec)。It can enable your Container Orchestrator to use Tencent Cloud storage。

## Version Support

| Kubernetes | Tencent Cloud CSI Version | branch |
| ------ | ------ | ------ |
| v1.10 | 0.3   |  release-0.3.0 |
| v1.11 | 0.3   |  release-0.3.0 |
| v1.12 | 0.3   |  release-0.3.0 |
| v1.13 | 1.0.0 | release-1.0.0  |
| v1.14+ | 1.1.0 | master        |


## CBS CSI Plugin

CBS provides elastic, efficient and reliable data storage. And it can be attached to one node at the same time. More detail please read [document](/docs/README_CBS.md).

## CFS CSI Plugin

Cloud File Storage (CFS) is a secure and scalable file sharing and storage solution. And it can be mounted by multi nodes at the same time. More detail please read [document](/docs/README_CFS.md).

## COSFS CSI Plugin

COSFS is a FUSE filesystem application backed by Tencent Cloud Object Storage(COS). COSFS will mount a COS bucket (that has been properly formatted) as a local file system. More detail please read [document](/docs/README_COSFS.md).

## Contributing

If you have any issues or would like to contribute, feel free to open an issue/PR.
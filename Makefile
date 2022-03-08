.PHONY: all

# image registry
REGISTRY?=ccr.ccs.tencentyun.com/tkeimages

# image tag
CBS_VERSION?=cbs
CBS_MULTI_VERSION?=cbs-multi
CFS_VERSION?=cfs
CFS_MULTI_VERSION?=cfs-multi
COS_VERSION?=cos
COS_MULTI_VERSION?=cos-multi
COS_LAUNCHER_VERSION?=cos-launcher
COS_LAUNCHER_MULTI_VERSION?=cos-launcher-multi
CHDFS_VERSION?=chdfs
CFSTURBO_VERSION?=cfsturbo

all: cbs cfs cos cfsturbo chdfs

cbs:
	docker build . --build-arg TARGETARCH=amd64 -f build/cbs/Dockerfile -t ${REGISTRY}/csi-tencentcloud-cbs:${CBS_VERSION}
	docker push ${REGISTRY}/csi-tencentcloud-cbs:${CBS_VERSION}
	docker buildx build --platform linux/amd64,linux/arm64 . -f build/cbs/Dockerfile -t ${REGISTRY}/csi-tencentcloud-cbs:${CBS_MULTI_VERSION} --push

cfs:
	docker build . --build-arg TARGETARCH=amd64 -f build/cfs/Dockerfile -t ${REGISTRY}/csi-tencentcloud-cfs:${CFS_VERSION}
	docker push ${REGISTRY}/csi-tencentcloud-cfs:${CFS_VERSION}
	docker buildx build --platform linux/amd64,linux/arm64 . -f build/cfs/Dockerfile -t ${REGISTRY}/csi-tencentcloud-cfs:${CFS_MULTI_VERSION} --push

cos:
	docker build . --build-arg TARGETARCH=amd64 -f build/cosfs/cosfs/Dockerfile -t ${REGISTRY}/csi-tencentcloud-cos:${COS_VERSION}
	docker build . --build-arg TARGETARCH=amd64 -f build/cosfs/launcher/Dockerfile -t ${REGISTRY}/csi-tencentcloud-cos-launcher:${COS_LAUNCHER_VERSION}
	docker push ${REGISTRY}/csi-tencentcloud-cos:${COS_VERSION}
	docker push ${REGISTRY}/csi-tencentcloud-cos-launcher:${COS_LAUNCHER_VERSION}
	docker buildx build --platform linux/amd64,linux/arm64 . -f build/cosfs/cosfs/Dockerfile -t ${REGISTRY}/csi-tencentcloud-cos:${COS_MULTI_VERSION} --push
	docker buildx build --platform linux/amd64,linux/arm64 . -f build/cosfs/launcher/Dockerfile -t ${REGISTRY}/csi-tencentcloud-cos-launcher:${COS_LAUNCHER_MULTI_VERSION} --push

chdfs:
	docker buildx build --platform linux/amd64,linux/arm64 . -f build/chdfs/Dockerfile -t ${REGISTRY}/csi-tencentcloud-chdfs:${CHDFS_VERSION} --push

cfsturbo:
	docker build . --build-arg TARGETARCH=amd64 -f build/cfsturbo/Dockerfile -t ${REGISTRY}/csi-tencentcloud-cfsturbo:${CFSTURBO_VERSION}
	docker push ${REGISTRY}/csi-tencentcloud-cfsturbo:${CFSTURBO_VERSION}

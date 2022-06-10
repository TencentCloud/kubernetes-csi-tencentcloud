.PHONY: all

# image registry
REGISTRY?=ccr.ccs.tencentyun.com/tkeimages

# cbs image tag
CBS_BASE?=base
CBS_VERSION?=cbs
CBS_MULTI_VERSION?=cbs-multi

# cfs image tag
CFS_VERSION?=cfs
CFS_MULTI_VERSION?=cfs-multi

# cos image tag
COS_VERSION?=cos
COS_MULTI_VERSION?=cos-multi
COS_LAUNCHER_VERSION?=cos-launcher
COS_LAUNCHER_MULTI_VERSION?=cos-launcher-multi

# chdfs image tag
CHDFS_VERSION?=chdfs

# cfsturbo image tag
CFSTURBO_VERSION?=cfsturbo

all: cbs cfs cos cfsturbo chdfs

cbs:
	sed -i "s/v1.0.0/${CBS_VERSION}/g" driver/cbs/driver.go
	docker build . --build-arg TARGETARCH=amd64 -f build/cbs/Dockerfile -t ${REGISTRY}/csi-tencentcloud-cbs:${CBS_VERSION}
	docker push ${REGISTRY}/csi-tencentcloud-cbs:${CBS_VERSION}

	sed -i "s/${CBS_VERSION}/${CBS_MULTI_VERSION}/g" driver/cbs/driver.go
	docker buildx build --platform linux/amd64,linux/arm64 . -f build/cbs/Dockerfile -t ${REGISTRY}/csi-tencentcloud-cbs:${CBS_MULTI_VERSION} --push

cbs-base:
	docker buildx build --platform linux/amd64,linux/arm64 . -f build/cbs/base/Dockerfile -t ${REGISTRY}/csi-tencentcloud-cbs:${CBS_BASE} --push

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

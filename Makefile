.PHONY: all

# image registry
REGISTRY?=ccr.ccs.tencentyun.com/tkeimages

# cbs image tag
CBS_BASE?=base
CBS_VERSION?=cbs
CBS_ARCH?=linux/amd64,linux/arm64

# cfs image tag
CFS_BASE?=base
CFS_VERSION?=cfs
CFS_ARCH?=linux/amd64,linux/arm64

# cos image tag
COS_VERSION?=cos
COS_MULTI_VERSION?=cos-multi

# cos-launcher image tag
COS_LAUNCHER_BASE?=base
COS_LAUNCHER_VERSION?=cos-launcher
COS_LAUNCHER_MULTI_VERSION?=cos-launcher-multi

# chdfs image tag
CHDFS_VERSION?=chdfs

# cfsturbo image tag
CFSTURBO_BASE?=base
CFSTURBO_VERSION?=cfsturbo

all: cbs cfs cos cfsturbo chdfs

cbs:
	sed -i "s/v1.0.0/${CBS_VERSION}/g" driver/cbs/driver.go
	docker buildx build --platform ${CBS_ARCH} . -f build/cbs/Dockerfile -t ${REGISTRY}/csi-tencentcloud-cbs:${CBS_VERSION} --push

cbs-base:
	docker buildx build --platform ${CBS_ARCH} . -f build/cbs/base/Dockerfile -t ${REGISTRY}/csi-tencentcloud-cbs:${CBS_BASE} --push

cfs:
	sed -i "s/v1.0.0/${CFS_VERSION}/g" driver/cfs/driver.go
	docker buildx build --platform ${CFS_ARCH} . -f build/cfs/Dockerfile -t ${REGISTRY}/csi-tencentcloud-cfs:${CFS_VERSION} --push

cfs-base:
	docker buildx build --platform linux/amd64,linux/arm64 . -f build/cfs/base/Dockerfile -t ${REGISTRY}/csi-tencentcloud-cfs:${CFS_BASE} --push

cos: cos-launcher
	sed -i "s/v1.0.0/${COS_VERSION}/g" driver/cosfs/driver.go
	docker build . --build-arg TARGETARCH=amd64 -f build/cosfs/cosfs/Dockerfile -t ${REGISTRY}/csi-tencentcloud-cos:${COS_VERSION}
	docker push ${REGISTRY}/csi-tencentcloud-cos:${COS_VERSION}

	sed -i "s/${COS_VERSION}/${COS_MULTI_VERSION}/g" driver/cosfs/driver.go
	docker buildx build --platform linux/amd64,linux/arm64 . -f build/cosfs/cosfs/Dockerfile -t ${REGISTRY}/csi-tencentcloud-cos:${COS_MULTI_VERSION} --push

cos-launcher:
	docker build . --build-arg TARGETARCH=amd64 -f build/cosfs/launcher/Dockerfile -t ${REGISTRY}/csi-tencentcloud-cos-launcher:${COS_LAUNCHER_VERSION}
	docker push ${REGISTRY}/csi-tencentcloud-cos-launcher:${COS_LAUNCHER_VERSION}
	docker buildx build --platform linux/amd64,linux/arm64 . -f build/cosfs/launcher/Dockerfile -t ${REGISTRY}/csi-tencentcloud-cos-launcher:${COS_LAUNCHER_MULTI_VERSION} --push

cos-launcher-base:
	docker buildx build --platform linux/amd64,linux/arm64 . -f build/cosfs/launcher/base/Dockerfile -t ${REGISTRY}/csi-tencentcloud-cos-launcher:${COS_LAUNCHER_BASE} --push

chdfs:
	sed -i "s/v1.0.0/${CHDFS_VERSION}/g" driver/chdfs/driver.go
	docker build . --build-arg TARGETARCH=amd64 -f build/chdfs/Dockerfile -t ${REGISTRY}/csi-tencentcloud-chdfs:${CHDFS_VERSION}
	docker push ${REGISTRY}/csi-tencentcloud-chdfs:${CHDFS_VERSION}

cfsturbo:
	sed -i "s/v1.0.0/${CFSTURBO_VERSION}/g" driver/cfsturbo/driver.go
	docker build . -f build/cfsturbo/Dockerfile -t ${REGISTRY}/csi-tencentcloud-cfsturbo:${CFSTURBO_VERSION}
	docker push ${REGISTRY}/csi-tencentcloud-cfsturbo:${CFSTURBO_VERSION}

cfsturbo-base:
	docker build . -f build/cfsturbo/base/Dockerfile -t ${REGISTRY}/csi-tencentcloud-cfsturbo:${CFSTURBO_BASE}
	docker push ${REGISTRY}/csi-tencentcloud-cfsturbo:${CFSTURBO_BASE}

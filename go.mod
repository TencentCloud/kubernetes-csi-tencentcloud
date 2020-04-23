module github.com/tencentcloud/kubernetes-csi-tencentcloud

go 1.13

require (
	github.com/container-storage-interface/spec v1.2.0
	github.com/dbdd4us/qcloudapi-sdk-go v0.0.0-20190530123522-c8d9381de48c
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.3.5
	github.com/gorilla/mux v1.7.4
	github.com/imdario/mergo v0.3.9 // indirect
	github.com/kubernetes-csi/csi-lib-utils v0.7.0 // indirect
	github.com/kubernetes-csi/drivers v1.0.2
	github.com/pkg/errors v0.9.1
	github.com/tencentcloud/tencentcloud-sdk-go v3.0.144+incompatible
	golang.org/x/net v0.0.0-20200324143707-d3edc9973b7e
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0 // indirect
	google.golang.org/grpc v1.28.0
	k8s.io/api v0.17.0
	k8s.io/apimachinery v0.17.1-beta.0
	k8s.io/client-go v0.17.0
	k8s.io/utils v0.0.0-20200327001022-6496210b90e8
)

/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cfsturbo

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/utils/mount"
)

const GB = 1 << (10 * 3)

type controllerServer struct {
	*csicommon.DefaultControllerServer
	mounter   mount.Interface
	clusterId string
}

func (cs *controllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	glog.Infof("CreateVolume CreateVolumeRequest is %v:", req)

	volumeCapabilities := req.GetVolumeCapabilities()
	name := req.GetName()
	if len(name) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume Name must be provided")
	}
	if len(volumeCapabilities) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume Volume capabilities must be provided")
	}

	parameters := req.GetParameters()
	opt := &cfsTurboOptions{}
	for key, value := range parameters {
		switch strings.ToLower(key) {
		case "fsid":
			opt.FSID = value
		case "host":
			opt.Server = value
		}
	}
	if opt.FSID == "" {
		return nil, status.Error(codes.InvalidArgument, "volumeAttributes's fsid should not empty")
	}
	if opt.Server == "" {
		return nil, status.Error(codes.InvalidArgument, "volumeAttributes's host should not empty")
	}
	if cs.clusterId == "" {
		return nil, status.Error(codes.InvalidArgument, "env CLUSTER_ID should not empty")
	}
	opt.RootDir = CFSTurboProtoNFSDefaultDIR

	mountPath := fmt.Sprintf("/tmp/%s", name)
	glog.Infof("mountPath: %v", mountPath)
	err := cs.mountCfsTurbo(mountPath, opt)
	if err != nil {
		return nil, err
	}

	err = os.MkdirAll(fmt.Sprintf("%s/%s/%s", mountPath, cs.clusterId, name), 0750)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	err = cs.mounter.Unmount(mountPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	parameters["path"] = fmt.Sprintf("/%s/%s", cs.clusterId, name)
	volumeId := fmt.Sprintf("%s-%s-%s", opt.FSID, opt.Server, parameters["path"])

	glog.Infof("CreateVolume %s success", name)

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volumeId,
			CapacityBytes: util.RoundUpGiB(req.GetCapacityRange().GetRequiredBytes()) * GB,
			VolumeContext: parameters,
		},
	}, nil
}

func (cs *controllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	glog.Infof("DeleteVolume DeleteVolumeRequest is %v:", req)

	volumeId := req.GetVolumeId()
	if len(volumeId) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}
	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid delete volume request: %v", req)
	}

	pvName, opt := getCfsTurboOptionsFromVolumeId(volumeId)
	if opt == nil || opt.FSID == "" || opt.Server == "" || pvName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Volume ID %s is invalid", volumeId)
	}

	mountPath := fmt.Sprintf("/tmp/%s", pvName)
	glog.Infof("mountPath: %v", mountPath)
	err := cs.mountCfsTurbo(mountPath, opt)
	if err != nil {
		return nil, err
	}

	err = os.RemoveAll(fmt.Sprintf("%s/%s/%s", mountPath, cs.clusterId, pvName))
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	err = cs.mounter.Unmount(mountPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	glog.Infof("DeleteVolume %s success", volumeId)

	return &csi.DeleteVolumeResponse{}, nil
}

func (cs *controllerServer) ControllerExpandVolume(context.Context, *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *controllerServer) mountCfsTurbo(mountPath string, opt *cfsTurboOptions) error {
	var mountSource string
	mo := make([]string, 0)
	opt.Proto = CFSTurboProtoLustre
	if opt.RootDir == "" {
		opt.RootDir = CFSTurboProtoNFSDefaultDIR
	}
	//check cfs lustre kernel mod install
	err := exec.Command("/bin/bash", "-c", fmt.Sprintf("lsmod | grep %s", CFSTurboLustreKernelModule)).Run()
	if err != nil {
		glog.Warning("Need install kernel mod in node before mount cfs turbo lustre, see https://cloud.tencent.com/document/product/582/54765")
	}

	mountSource = fmt.Sprintf("%s@tcp0:/%s%s", opt.Server, opt.FSID, opt.RootDir)
	glog.Infof("CFS server %s mount option is: %v", mountSource, mo)

	notMnt, err := cs.mounter.IsLikelyNotMountPoint(mountPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(mountPath, 0750); err != nil {
				return status.Error(codes.Internal, err.Error())
			}
			notMnt = true
		} else {
			return status.Error(codes.Internal, err.Error())
		}
	}
	if notMnt {
		err = cs.mounter.Mount(mountSource, mountPath, opt.Proto, make([]string, 0))
		if err != nil {
			if os.IsPermission(err) {
				return status.Error(codes.PermissionDenied, err.Error())
			}
			if strings.Contains(err.Error(), "invalid argument") {
				return status.Error(codes.InvalidArgument, err.Error())
			}
			return status.Error(codes.Internal, err.Error())
		}
	}

	return nil
}

func getCfsTurboOptionsFromVolumeId(volumeId string) (string, *cfsTurboOptions) {
	opt := &cfsTurboOptions{}
	options := strings.Split(volumeId, "-")
	if len(options) < 3 {
		return "", opt
	}
	opt.FSID = options[0]
	opt.Server = options[1]
	path := strings.Join(options[2:], "-")
	return strings.Split(path, "/")[2], opt
}

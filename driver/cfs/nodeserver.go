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

package cfs

import (
	"fmt"
	"os"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi/v0"
	"github.com/golang/glog"
	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/utils"
	"k8s.io/kubernetes/pkg/util/mount"

	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
)

type nodeServer struct {
	*csicommon.DefaultNodeServer
	mounter mount.Interface
}

type cfsOptions struct {
	Server  string `json:"host"`
	Path    string `json:"path"`
	Vers    string `json:"vers"`
	Options string `json:"options"`
	FSID    string `json:"fsid"`
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {

	glog.Infof("NodePublishVolume NodePublishVolumeRequest is: %v", req)

	// parse parameters
	mountPath := req.GetTargetPath()
	opt := &cfsOptions{}

	for key, value := range req.VolumeAttributes {
		switch strings.ToLower(key) {
		case "host":
			opt.Server = value
		case "path":
			opt.Path = value
		case "vers":
			opt.Vers = value
		case "options":
			opt.Options = value
		case "fsid":
			opt.FSID = value
		}
	}

	// check parameters
	if mountPath == "" {
		return nil, status.Error(codes.InvalidArgument, "req.GetTargetPath() is empty")
	}
	if opt.Server == "" {
		return nil, status.Error(codes.InvalidArgument, "volumeAttributes's host should not empty")
	}

	//check path
	if opt.Path == "" {
		opt.Path = "/"
	}

	if !strings.HasPrefix(opt.Path, "/") {
		return nil, status.Error(codes.InvalidArgument, "volumeAttributes's path format is illegal")
	}

	//check version
	if opt.Vers == "" {
		opt.Vers = "4"
	}

	notMnt, err := ns.mounter.IsLikelyNotMountPoint(mountPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(mountPath, 0750); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			notMnt = true
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if !notMnt {
		return &csi.NodePublishVolumeResponse{}, nil
	}

	mo := req.GetVolumeCapability().GetMount().GetMountFlags()
	if req.GetReadonly() {
		mo = append(mo, "ro")
	}

	if opt.Options != "" {
		mo = append(mo, strings.Split(opt.Options, ",")...)
	}
	mo = append(mo, fmt.Sprintf("vers=%s", opt.Vers))

	// cfs use nfs v3 protocol need extra mount option
	if opt.Vers == "3" {
		opt.Path = fmt.Sprintf("/%s%s", opt.FSID, opt.Path)
		mo = append(mo, "nolock,proto=tcp")
	}
	source := fmt.Sprintf("%s:%s", opt.Server, opt.Path)

	glog.Infof("CFS server %s mount option is: %v", source, mo)

	err = ns.mounter.Mount(source, mountPath, "nfs", mo)
	if err != nil {
		if os.IsPermission(err) {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
		if strings.Contains(err.Error(), "invalid argument") {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {

	glog.Infof("NodeUnpublishVolume NodeUnpublishVolumeRequest is: %v", req)

	targetPath := req.GetTargetPath()
	notMnt, err := ns.mounter.IsLikelyNotMountPoint(targetPath)

	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Error(codes.NotFound, "Targetpath not found")
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	if notMnt {
		return nil, status.Error(codes.NotFound, "Volume not mounted")
	}

	err = utils.CleanupMountPoint(req.GetTargetPath(), ns.mounter, false)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeStageVolume(
	ctx context.Context,
	req *csi.NodeStageVolumeRequest) (
	*csi.NodeStageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (ns *nodeServer) NodeUnstageVolume(
	ctx context.Context,
	req *csi.NodeUnstageVolumeRequest) (
	*csi.NodeUnstageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

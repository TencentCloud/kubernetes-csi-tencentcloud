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
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi/v0"
	"github.com/golang/glog"
	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/utils"
	"k8s.io/kubernetes/pkg/util/mount"

	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
)

var (
	nodeCaps = []csi.NodeServiceCapability_RPC_Type{
		csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
	}
)

const (
	nfsPort = "2049"
	// CFSTurboProtoLustre ...
	CFSTurboProtoLustre = "lustre"
	// CFSTurboProtoNFS ...
	CFSTurboProtoNFS = "nfs"
	// CFSTurboProtoNFSDefaultDIR ...
	CFSTurboProtoNFSDefaultDIR = "cfs"
	// CFSTurboLustreKernelModule ...
	CFSTurboLustreKernelModule = "ptlrpc"
)

type nodeServer struct {
	*csicommon.DefaultNodeServer
	mounter mount.Interface
}

type cfsTurboOptions struct {
	Proto   string `json:"proto"`
	Server  string `json:"host"`
	Path    string `json:"path"`
	Options string `json:"options"`
	FSID    string `json:"fsid"`
}

func (ns *nodeServer) NodeStageVolume(
	ctx context.Context,
	req *csi.NodeStageVolumeRequest) (
	*csi.NodeStageVolumeResponse, error) {
	glog.Infof("NodeStageVolume NodeStageVolumeRequest is: %v", req)

	// common  parameters
	mountPath := req.GetStagingTargetPath()
	if mountPath == "" {
		return nil, status.Error(codes.InvalidArgument, "req.GetStagingTargetPath() is empty")
	}

	opt := &cfsTurboOptions{}

	for key, value := range req.GetVolumeAttributes() {
		switch strings.ToLower(key) {
		case "host":
			opt.Server = value
		case "options":
			opt.Options = value
		case "fsid":
			opt.FSID = value
		case "proto":
			opt.Proto = value
		}
	}
	// check protocol first
	if opt.Proto == "" {
		opt.Proto = CFSTurboProtoLustre
	}

	if opt.FSID == "" {
		return nil, status.Error(codes.InvalidArgument, "volumeAttributes's fsid should not empty")
	}
	if opt.Server == "" {
		return nil, status.Error(codes.InvalidArgument, "volumeAttributes's host should not empty")
	}

	mo := req.GetVolumeCapability().GetMount().GetMountFlags()
	if opt.Options != "" {
		mo = append(mo, opt.Options)
	}

	var mountSource string

	switch opt.Proto {
	case CFSTurboProtoNFS:
		// check nfs parameters and connection
		// check network connection
		conn, err := net.DialTimeout("tcp", opt.Server+":"+nfsPort, time.Second*time.Duration(3))
		if err != nil {
			glog.Errorf("CFSTurbo: Cannot connect to nfs host: %s", opt.Server)
			return nil, errors.New("CFSTurbo: Cannot connect to nfs host: " + opt.Server)
		}
		conn.Close()

		// cfs turbo need use nfs v3
		mo = append(mo, "vers=3,nolock,noresvport")
		// cfs turbo mount node use fsid
		mountSource = fmt.Sprintf("%s:/%s", opt.Server, opt.FSID)

	case CFSTurboProtoLustre:
		//check cfs lustre core kmod install
		err := exec.Command("/bin/bash", "-c", fmt.Sprintf("lsmod | grep %s", CFSTurboLustreKernelModule)).Run()
		if err != nil {
			return nil, status.Error(codes.Unavailable, "Need install kernel mod in node before mount cfs turbo lustre, see https://cloud.tencent.com/document/product/582/54765")
		}
		mountSource = fmt.Sprintf("%s@tcp0:/%s", opt.Server, opt.FSID)
	default:
		return nil, status.Error(codes.InvalidArgument, "Unsupport protocol type")
	}
	glog.Infof("CFS server %s mount option is: %v", mountSource, mo)

	//check mountPath
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
		return &csi.NodeStageVolumeResponse{}, nil
	}

	err = ns.mounter.Mount(mountSource, mountPath, opt.Proto, mo)
	if err != nil {
		if os.IsPermission(err) {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
		if strings.Contains(err.Error(), "invalid argument") {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnstageVolume(
	ctx context.Context,
	req *csi.NodeUnstageVolumeRequest) (
	*csi.NodeUnstageVolumeResponse, error) {
	glog.Infof("NodeUnstageVolume NodeUnstageVolumeRequest is: %v", req)

	targetPath := req.GetStagingTargetPath()
	notMnt, err := ns.mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Error(codes.NotFound, "Targetpath not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	if notMnt {
		return &csi.NodeUnstageVolumeResponse{}, nil
	}

	err = utils.CleanupMountPoint(req.GetStagingTargetPath(), ns.mounter, false)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {

	glog.Infof("NodePublishVolume NodePublishVolumeRequest is: %v", req)

	stagingTargetPath := req.GetStagingTargetPath()
	if stagingTargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "req.GetStagingTargetPath() is empty is empty")
	}
	targetPath := req.GetTargetPath()
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "targetPath is empty")
	}

	opt := &cfsTurboOptions{}
	for key, value := range req.GetVolumeAttributes() {
		switch strings.ToLower(key) {
		case "path":
			opt.Path = value
		case "proto":
			opt.Proto = value
		}
	}
	mo := req.VolumeCapability.GetMount().MountFlags
	mo = append(mo, "bind")

	if req.Readonly {
		mo = append(mo, "ro")
	}

	// check protocol first
	if opt.Proto == "" {
		opt.Proto = CFSTurboProtoLustre
	}
	//check path
	if opt.Path == "" {
		opt.Path = "/"
	}
	if !strings.HasPrefix(opt.Path, "/") {
		return nil, status.Error(codes.InvalidArgument, "volumeAttributes's path prefix must be /")
	}

	// get global mount sub directory bind mount

	mountSource := fmt.Sprintf("%s/%s%s", stagingTargetPath, CFSTurboProtoNFSDefaultDIR, opt.Path)

	if _, err := os.Stat(targetPath); err != nil {
		if os.IsNotExist(err) {
			err := os.MkdirAll(targetPath, 0750)
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if err := ns.mounter.Mount(mountSource, targetPath, opt.Proto, mo); err != nil {
		glog.Errorf("NodePublishVolume: Mount error target %v error %v", targetPath, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {

	glog.Infof("NodeUnpublishVolume NodeUnpublishVolumeRequest is: %v", req)

	targetPath := req.GetTargetPath()
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "req.GetTargetPath() is empty")
	}
	notMnt, err := ns.mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Error(codes.NotFound, "Targetpath not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	if notMnt {
		return &csi.NodeUnpublishVolumeResponse{}, nil
	}

	if err := ns.mounter.Unmount(targetPath); err != nil {
		glog.Errorf("NodeUnpublishVolume: Mount error targetPath %v error %v", targetPath, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	glog.Infof("NodeGetCapabilities: called with args %+v", *req)
	var caps []*csi.NodeServiceCapability
	for _, cap := range nodeCaps {
		c := &csi.NodeServiceCapability{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: cap,
				},
			},
		}
		caps = append(caps, c)
	}
	return &csi.NodeGetCapabilitiesResponse{Capabilities: caps}, nil
}

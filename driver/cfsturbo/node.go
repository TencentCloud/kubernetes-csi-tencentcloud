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

/*
CFS turbo is a parallel file system that supports many requirements of leadership class HPC simulation environments.
It supports nfs v3 protocol and lustre protocol. CFS turbo NFSv3 need mount {server}:/{fsid} to node, and bind mount
{server}:/{fsid}/cfs to container mount ns. The "cfs" directory is hard limit, directory which is created by user is
the sub-directory for it.
1. mount -t nfs  1.1.1.1:/fsid  /mnt
2. mount --bind  /mnt/cfs  {container mount ns}
So we mount {server}:/{fsid} to global mount directory in NodeStageVolume, and bind mount stagepath+"/cfs" to pod mount
directory in NodePublishVolume.
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

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/util"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/utils/mount"
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
	CFSTurboProtoNFSDefaultDIR = "/cfs"
	// CFSTurboLustreKernelModule ...
	CFSTurboLustreKernelModule = "ptlrpc"
)

type nodeServer struct {
	*csicommon.DefaultNodeServer
	VolumeLocks *util.VolumeLocks
	mounter     mount.Interface
}

type cfsTurboOptions struct {
	Proto   string `json:"proto"`
	Server  string `json:"host"`
	Path    string `json:"path"`
	Options string `json:"options"`
	FSID    string `json:"fsid"`
	RootDir string `json:"rootdir"`
}

func (ns *nodeServer) NodeStageVolume(
	ctx context.Context,
	req *csi.NodeStageVolumeRequest) (
	*csi.NodeStageVolumeResponse, error) {
	glog.Infof("NodeStageVolume NodeStageVolumeRequest is: %v", req)

	// common  parameters
	opt := &cfsTurboOptions{}
	for key, value := range req.GetVolumeContext() {
		switch strings.ToLower(key) {
		case "proto":
			opt.Proto = value
		case "rootdir":
			opt.RootDir = value
		case "fsid":
			opt.FSID = value
		case "host":
			opt.Server = value
		case "options":
			opt.Options = value
		}
	}

	// check protocol first
	if opt.Proto == "" {
		opt.Proto = CFSTurboProtoLustre
	}
	if opt.RootDir == "" {
		opt.RootDir = CFSTurboProtoNFSDefaultDIR
	}
	if !strings.HasPrefix(opt.RootDir, "/") {
		return nil, status.Error(codes.InvalidArgument, "volumeAttributes's rootdir format is illegal")
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

	fsidWithRootDir := opt.FSID
	if opt.RootDir != CFSTurboProtoNFSDefaultDIR {
		fsidWithRootDir = opt.FSID + strings.ReplaceAll(opt.RootDir, "/", "-")
	}

	if acquired := ns.VolumeLocks.TryAcquire(fsidWithRootDir); !acquired {
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, fsidWithRootDir)
	}
	defer ns.VolumeLocks.Release(fsidWithRootDir)

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
		mountSource = fmt.Sprintf("%s:/%s%s", opt.Server, opt.FSID, opt.RootDir)

	case CFSTurboProtoLustre:
		//check cfs lustre core kmod install
		err := exec.Command("/bin/bash", "-c", fmt.Sprintf("lsmod | grep %s", CFSTurboLustreKernelModule)).Run()
		if err != nil {
			glog.Warning(codes.Unavailable, "Need install kernel mod in node before mount cfs turbo lustre, see https://cloud.tencent.com/document/product/582/54765")
		}
		mountSource = fmt.Sprintf("%s@tcp0:/%s%s", opt.Server, opt.FSID, opt.RootDir)
	default:
		return nil, status.Error(codes.InvalidArgument, "Unsupport protocol type")
	}
	glog.Infof("CFS server %s mount option is: %v", mountSource, mo)

	mountPath := fmt.Sprintf("%s/%s", cfsturboGlobalPath, fsidWithRootDir)
	glog.Infof("Global mountPath: %v", mountPath)

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
		err = AddVolumeIdToCfsturboConfig(fsidWithRootDir, req.GetVolumeId())
		if err != nil {
			glog.Errorf("AddVolumeIdToCfsturboConfig failed, err: %v", err)
			return nil, status.Error(codes.Internal, err.Error())
		}
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

	err = AddVolumeIdToCfsturboConfig(fsidWithRootDir, req.GetVolumeId())
	if err != nil {
		glog.Errorf("AddVolumeIdToCfsturboConfig failed, err: %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnstageVolume(
	ctx context.Context,
	req *csi.NodeUnstageVolumeRequest) (
	*csi.NodeUnstageVolumeResponse, error) {
	glog.Infof("NodeUnstageVolume NodeUnstageVolumeRequest is: %v", req)

	fsidWithRootDir, err := GetFSIDWithRootDirByVolumeId(req.GetVolumeId())
	if err != nil {
		glog.Errorf("Get fsidWithRootDir from cfsturboConfigName failed, err: %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	if fsidWithRootDir == "" {
		glog.Warningf("FSIDWithRootDir is empty, skip node unstage")
		return &csi.NodeUnstageVolumeResponse{}, nil
	}

	if acquired := ns.VolumeLocks.TryAcquire(fsidWithRootDir); !acquired {
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, fsidWithRootDir)
	}
	defer ns.VolumeLocks.Release(fsidWithRootDir)

	needUmount, err := DeleteVolumeIdFromCfsturboConfig(req.GetVolumeId(), fsidWithRootDir)
	if err != nil {
		glog.Errorf("DeleteVolumeIdFromCfsturboConfig failed, err: %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	if !needUmount {
		glog.Infof("FSIDWithRootDir %s is still in use, skip node unstage", fsidWithRootDir)
		return &csi.NodeUnstageVolumeResponse{}, nil
	}

	mountPath := fmt.Sprintf("%s/%s", cfsturboGlobalPath, fsidWithRootDir)
	err = mount.CleanupMountPoint(mountPath, ns.mounter, false)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	err = DeleteCfsturboConfig(fsidWithRootDir)
	if err != nil {
		glog.Errorf("DeleteCfsturboConfig failed, err: %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	glog.Infof("NodePublishVolume NodePublishVolumeRequest is: %v", req)

	targetPath := req.GetTargetPath()
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "targetPath is empty")
	}

	opt := &cfsTurboOptions{}
	for key, value := range req.GetVolumeContext() {
		switch strings.ToLower(key) {
		case "proto":
			opt.Proto = value
		case "rootdir":
			opt.RootDir = value
		case "fsid":
			opt.FSID = value
		case "path":
			opt.Path = value
		}
	}

	// check protocol first
	if opt.Proto == "" {
		opt.Proto = CFSTurboProtoLustre
	}
	if opt.RootDir == "" {
		opt.RootDir = CFSTurboProtoNFSDefaultDIR
	}
	if !strings.HasPrefix(opt.RootDir, "/") {
		return nil, status.Error(codes.InvalidArgument, "volumeAttributes's rootdir format is illegal")
	}
	if opt.FSID == "" {
		return nil, status.Error(codes.InvalidArgument, "volumeAttributes's fsid should not empty")
	}
	//check path
	if opt.Path == "" {
		opt.Path = "/"
	}
	if !strings.HasPrefix(opt.Path, "/") {
		return nil, status.Error(codes.InvalidArgument, "volumeAttributes's path prefix must be /")
	}
	mo := req.VolumeCapability.GetMount().MountFlags
	mo = append(mo, "bind")
	if req.Readonly {
		mo = append(mo, "ro")
	}

	fsidWithRootDir := opt.FSID
	if opt.RootDir != CFSTurboProtoNFSDefaultDIR {
		fsidWithRootDir = opt.FSID + strings.ReplaceAll(opt.RootDir, "/", "-")
	}

	if err := ns.CheckGlobalMountPath(fsidWithRootDir, ctx, req); err != nil {
		glog.Errorf("NodePublishVolume: CheckGlobalMountPath failed, error: %v", err)
		return nil, err
	}

	notMnt, err := ns.mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			err := os.MkdirAll(targetPath, 0750)
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	if !notMnt {
		glog.Infof("NodePublishVolume: TargetPath %s is already mounted, skipping", targetPath)
		return &csi.NodePublishVolumeResponse{}, nil
	}

	mountSource := fmt.Sprintf("%s/%s%s", cfsturboGlobalPath, fsidWithRootDir, opt.Path)
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

	for n := 0; ; n++ {
		notMnt, err := ns.mounter.IsLikelyNotMountPoint(targetPath)
		if err != nil {
			if os.IsNotExist(err) {
				glog.Infof("target path not exist, it's reasonable to assume that the `NodeUnpublishVolume` is successful.")
				return &csi.NodeUnpublishVolumeResponse{}, nil
			}
			return nil, status.Error(codes.Internal, err.Error())
		}
		if notMnt {
			if n == 0 {
				glog.Infof("NodeUnpublishVolume: targetPath %v is already unmounted", targetPath)
				return &csi.NodeUnpublishVolumeResponse{}, nil
			}
			glog.Infof("NodeUnpublishVolume: umount targetPath %v for %d times", targetPath, n)
			break
		}

		if err := ns.mounter.Unmount(targetPath); err != nil {
			glog.Errorf("NodeUnpublishVolume: Mount error targetPath %v error %v", targetPath, err)
			return nil, status.Error(codes.Internal, err.Error())
		}
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

func (ns *nodeServer) NodeExpandVolume(context.Context, *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "NodeExpandVolume is not implemented yet")
}

func (ns *nodeServer) CheckGlobalMountPath(fsidWithRootDir string, ctx context.Context, req *csi.NodePublishVolumeRequest) error {
	mountPath := fmt.Sprintf("%s/%s", cfsturboGlobalPath, fsidWithRootDir)

	notMnt, err := ns.mounter.IsLikelyNotMountPoint(mountPath)
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
		glog.Infof("Call NodeStageVolume to provide the global mountPath of %s", fsidWithRootDir)
		stageReq := &csi.NodeStageVolumeRequest{
			VolumeId:         req.GetVolumeId(),
			VolumeCapability: req.GetVolumeCapability(),
			VolumeContext:    req.GetVolumeContext(),
		}

		_, err := ns.NodeStageVolume(ctx, stageReq)
		if err != nil {
			return err
		}
	}

	return nil
}

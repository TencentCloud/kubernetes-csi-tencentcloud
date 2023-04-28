package chdfs

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/utils/mount"
)

type nodeServer struct {
	*csicommon.DefaultNodeServer
	mounter mount.Interface
}

type chdfsOptions struct {
	AllowOther     bool
	IsSync         bool
	IsDebug        bool
	Url            string
	AdditionalArgs string
}

func (node *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (node *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (node *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	glog.Infof("NodePublishVolume NodePublishVolumeRequest is: %v", req)

	volID := req.GetVolumeId() //this should be pv name
	if volID == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID missing in request")
	}
	targetPath := req.GetTargetPath()
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "target path missing in request")
	}
	options := parseCHDFSOptions(req.GetVolumeContext())
	if options.Url == "" {
		return nil, status.Error(codes.InvalidArgument, "url missing in request")
	}

	isMnt, err := createMountPoint(volID, targetPath)
	if err != nil {
		return nil, err
	}
	if isMnt {
		glog.Infof("Volume %s is already mounted to %s, skipping", volID, targetPath)
		return &csi.NodePublishVolumeResponse{}, nil
	}

	if err := Mount(options, targetPath); err != nil {
		glog.Errorf("Mount %s to %s failed: %v", volID, targetPath, err)
		return nil, status.Errorf(codes.Internal, "mount failed: %v", err)
	}

	glog.Infof("Successfully mounted volume %s to %s", volID, targetPath)

	return &csi.NodePublishVolumeResponse{}, nil
}

func (node *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	glog.Infof("NodeUnpublishVolume NodeUnpublishVolumeRequest is: %v", req)

	volID := req.GetVolumeId()
	if volID == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID missing in request")
	}
	targetPath := req.GetTargetPath()
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "target path missing in request")
	}

	notMnt, err := node.mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			glog.Infof("target path %s already deleted", targetPath)
			return &csi.NodeUnpublishVolumeResponse{}, nil
		} else if strings.Contains(err.Error(), "transport endpoint is not connected") {
			notMnt = false
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	if notMnt {
		glog.Infof("target path %s already unmounted", targetPath)
		return &csi.NodeUnpublishVolumeResponse{}, nil
	}

	if err := node.mounter.Unmount(targetPath); err != nil {
		glog.Errorf("Failed to umount point %s for volume %s: %v", targetPath, volID, err)
		return nil, status.Errorf(codes.Internal, "umount failed: %v", err)
	}

	glog.Infof("Successfully unmounted volume %s from %s", req.GetVolumeId(), targetPath)

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (node *nodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return &csi.NodeExpandVolumeResponse{}, nil
}

func parseCHDFSOptions(attributes map[string]string) *chdfsOptions {
	options := &chdfsOptions{}
	for k, v := range attributes {
		switch strings.ToLower(k) {
		case "allowother":
			options.AllowOther = isTrue(v)
		case "sync":
			options.IsSync = isTrue(v)
		case "debug":
			options.IsDebug = isTrue(v)
		case "url":
			options.Url = v
		case "additional_args":
			options.AdditionalArgs = v
		}
	}

	return options
}

func isTrue(str string) bool {
	isTrue, err := strconv.ParseBool(str)
	if err != nil {
		return false
	}
	return isTrue
}

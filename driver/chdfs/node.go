package chdfs

import (
	"context"
	"errors"
	"fmt"
	"k8s.io/utils/exec"
	"k8s.io/utils/mount"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// Used to create chdfs config file.
	perm = 0600

	SocketPath = "/tmp/chdfs.sock"
	ConfigPath = "/etc/chdfs"
)

var (
	nodeCaps = []csi.NodeServiceCapability_RPC_Type{
		csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
	}
)

func newNodeServer(driver *csicommon.CSIDriver, mounter mounter) csi.NodeServer {
	return &nodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(driver),
		mounter: mount.SafeFormatAndMount{
			Interface: mount.New(""),
			Exec:      exec.New(),
		},
		localMounter: mounter,
	}
}

type nodeServer struct {
	*csicommon.DefaultNodeServer
	localMounter mounter
	mounter      mount.SafeFormatAndMount
}

type chdfsOptions struct {
	AllowOther          bool
	IsSync              bool
	IsDebug             bool
	ConfigMapNamespaces string
	ConfigMapName       string
}

func (node *nodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
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

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	if err := validateNodePublishVolumeRequest(req); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	volID := req.GetVolumeId()
	targetPath := req.GetTargetPath()

	options, err := parseCHDFSOptions(req.GetVolumeContext())
	if err != nil {
		glog.Errorf("parse options from VolumeAttributes for %s failed: %v", volID, err)
		return nil, status.Errorf(codes.InvalidArgument, "parse options failed: %v", err)
	}

	isMnt, err := ns.createMountPoint(volID, targetPath)
	if err != nil {
		return nil, err
	}
	if isMnt {
		glog.Infof("Volume %s is already mounted to %s, skipping", volID, targetPath)
		return &csi.NodePublishVolumeResponse{}, nil
	}

	// Create chdfs config file
	config, configPath, err := ns.createConfigFile(options)
	if err != nil {
		return nil, err
	}

	if err := ns.localMounter.Mount(options, targetPath, config, configPath); err != nil {
		glog.Errorf("Mount %s to %s failed: %v", volID, targetPath, err)
		return nil, status.Errorf(codes.Internal, "mount failed: %v", err)
	}

	glog.Infof("Successfully mounted volume %s to %s", volID, targetPath)

	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	if err := validateNodeUnpublishVolumeRequest(req); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	volID := req.GetVolumeId()
	targetPath := req.GetTargetPath()

	// use launcher
	if err := ns.localMounter.Umount(targetPath); err != nil {
		glog.Errorf("Failed to umount point %s for volume %s: %v", targetPath, volID, err)
		return nil, status.Errorf(codes.Internal, "umount failed: %v", err)
	}

	glog.Infof("Successfully unmounted volume %s from %s", req.GetVolumeId(), targetPath)

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (node *nodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	glog.Infof("NodeExpandVolume: NodeExpandVolumeRequest is %#v", *req)

	//volumeID := req.GetVolumeId()
	//if len(volumeID) == 0 {
	//	return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	//}
	//
	//args := []string{"-o", "source", "--noheadings", "--target", req.GetVolumePath()}
	//output, err := node.mounter.Exec.Command("findmnt", args...).Output()
	//if err != nil {
	//	return nil, status.Errorf(codes.Internal, "Could not determine device path: %v, raw block device or unmounted", err)
	//}
	//
	//devicePath := strings.TrimSpace(string(output))
	//if len(devicePath) == 0 {
	//	return nil, status.Errorf(codes.Internal, "Could not get valid device for mount path: %v", req.GetVolumePath())
	//}
	//r := resizefs.NewResizeFs(&node.mounter)
	//if _, err := r.Resize(devicePath, req.GetVolumePath()); err != nil {
	//	return nil, status.Errorf(codes.Internal, "Could not resize volume %s %s:  %v", volumeID, devicePath, err)
	//}

	return &csi.NodeExpandVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func isFileExisted(filename string) bool {
	_, err := os.Stat(filename)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return true
}

func createMountPoint(point string) error {
	return os.MkdirAll(point, perm)
}

func isMountPoint(path string) (bool, error) {
	mounter := mount.New("")
	notMnt, err := mounter.IsLikelyNotMountPoint(path)
	if err != nil {
		return false, err
	}
	return !notMnt, nil
}

func (ns *nodeServer) createMountPoint(volID, targetPath string) (bool, error) {
	glog.Infof("Creating staging mount point at %s for volume %s", targetPath, volID)
	if !isFileExisted(targetPath) {
		glog.Infof("staging mount point path %s is not exist.\n", targetPath)
		if err := createMountPoint(targetPath); err != nil {
			glog.Errorf("failed to create staging mount point at %s: %v", targetPath, err)
			return false, err
		}
	}
	isMounted, err := isMountPoint(targetPath)
	if err != nil {
		glog.Error("Error in checkout is mount point: ", err)
		return false, err
	}
	return isMounted, nil
}

func parseCHDFSOptions(attributes map[string]string) (*chdfsOptions, error) {
	options := &chdfsOptions{
		AllowOther: false,
		IsSync:     false,
		IsDebug:    false,
	}
	for k, v := range attributes {
		switch strings.ToLower(k) {
		case "allowother":
			options.AllowOther = isTrue(v)
		case "sync":
			options.IsSync = isTrue(v)
		case "debug":
			options.IsDebug = isTrue(v)
		case "configmapname":
			options.ConfigMapName = v
		case "configmapnamespaces":
			options.ConfigMapNamespaces = v
		}
	}
	return options, validateCHDFSOptions(options)
}

func isTrue(str string) bool {
	isTrue, err := strconv.ParseBool(str)
	if err != nil {
		return false
	}
	return isTrue
}

func validateCHDFSOptions(options *chdfsOptions) error {
	if options.ConfigMapName == "" {
		return errors.New("CHDFS service configmap can't be empty")
	}
	if options.ConfigMapNamespaces == "" {
		return errors.New("CHDFS service configmap namespaces can't be empty")
	}
	return nil
}

func (ns *nodeServer) createConfigFile(options *chdfsOptions) (string, string, error) {
	configMapName := options.ConfigMapName
	namespace := options.ConfigMapNamespaces
	// Create new client
	client, err := NewK8sClient()
	if err != nil {
		return "", "", status.Errorf(codes.Internal, "get config failed: %v", err)
	}

	// Fetch config by client-go
	configMap, err := client.GetConfigMap(configMapName, namespace)
	if err != nil {
		glog.Errorf("Fetch configMap from api server failed: %v", err)
		return "", "", status.Errorf(codes.InvalidArgument, "get config failed: %v", err)
	}

	configFileName := fmt.Sprintf("csi_chdfs_%s_%s.toml", configMapName, namespace)
	if _, exist := configMap.Data["chdfs"]; !exist {
		glog.Errorf("No key <chdfs> found in configMap %s", configMapName)
		return "", "", status.Errorf(codes.InvalidArgument, "get config failed: %v", err)
	}

	chdfsConfig := configMap.Data["chdfs"]
	chdfsConfigPath := path.Join(ConfigPath, configFileName)

	return chdfsConfig, chdfsConfigPath, nil
}

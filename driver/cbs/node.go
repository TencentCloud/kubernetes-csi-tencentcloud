package cbs

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/pkg/util/resizefs"
	"k8s.io/kubernetes/pkg/volume"
	"k8s.io/utils/exec"
	"k8s.io/utils/mount"

	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/metrics"
	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/util"
)

const (
	DiskByIDDevicePath       = "/dev/disk/by-id"
	DiskByIDDeviceNamePrefix = "virtio-"
	NodeNameKey              = "NODE_ID"

	defaultMaxAttachVolumePerNode = 18
)

var (
	nodeCaps = []csi.NodeServiceCapability_RPC_Type{
		csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
		csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
		csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
	}
)

type cbsNode struct {
	mounter    mount.SafeFormatAndMount
	idempotent *util.Idempotent

	zone              string
	nodeID            string
	volumeAttachLimit int64
}

func newCbsNode(drv *Driver) *cbsNode {
	if drv.nodeID == "" {
		nodeID := getInstanceIdFromProviderID(drv.client)
		if nodeID == "" {
			glog.Fatal("get instanceID from node failed")
		}
		glog.Infof("instanceID: %s", nodeID)
		drv.nodeID = nodeID
	}

	return &cbsNode{
		mounter: mount.SafeFormatAndMount{
			Interface: mount.New(""),
			Exec:      exec.New(),
		},
		idempotent:        util.NewIdempotent(),
		zone:              drv.zone,
		nodeID:            drv.nodeID,
		volumeAttachLimit: drv.volumeAttachLimit,
	}
}

func (node *cbsNode) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	glog.Infof("NodeStageVolume: start with args %v", *req)

	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume id is empty")
	}
	if req.StagingTargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "volume staging target path is empty")
	}
	if req.VolumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "volume has no capabilities")
	}
	// cbs is not support rawblock currently
	if req.VolumeCapability.GetMount() == nil {
		return nil, status.Error(codes.InvalidArgument, "volume access type is not mount")
	}

	// 1. check if current req is in progress.
	if ok := node.idempotent.Insert(req); !ok {
		msg := fmt.Sprintf("volume %v is in progress", req.VolumeId)
		return nil, status.Error(codes.Internal, msg)
	}

	defer func() {
		glog.Infof("NodeStageVolume: volume %v finished", req.VolumeId)
		node.idempotent.Delete(req)
	}()

	diskID := req.VolumeId

	stagingTargetPath := req.StagingTargetPath

	mountFlags := req.VolumeCapability.GetMount().MountFlags
	mountFsType := req.VolumeCapability.GetMount().FsType

	if _, err := os.Stat(stagingTargetPath); err != nil {
		if os.IsNotExist(err) {
			err := os.MkdirAll(stagingTargetPath, 0750)
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	//2. check target path mounted
	diskSource, err := findCBSVolume(diskID)
	if err != nil {
		glog.Infof("NodeStageVolume: findCBSVolume error cbs disk=%v, error %v", filepath.Join(DiskByIDDevicePath, DiskByIDDeviceNamePrefix+diskID), err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	device, _, err := mount.GetDeviceNameFromMount(node.mounter, stagingTargetPath)
	if err != nil {
		glog.Errorf("NodeStageVolume: GetDeviceNameFromMount error %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	if diskSource == device || filepath.Join(DiskByIDDevicePath, DiskByIDDeviceNamePrefix+diskID) == device {
		glog.Infof("NodeStageVolume: volume %v already staged", diskID)
		return &csi.NodeStageVolumeResponse{}, nil
	}

	if err := node.mounter.FormatAndMount(diskSource, stagingTargetPath, mountFsType, mountFlags); err != nil {
		glog.Errorf(
			"NodeStageVolume: FormatAndMount error diskSource %v stagingTargetPath %v, error %v",
			diskSource, stagingTargetPath, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	r := resizefs.NewResizeFs(&node.mounter)
	if _, err := r.Resize(diskSource, stagingTargetPath); err != nil && !isNotSupport(err) {
		return nil, status.Errorf(codes.Internal, "NodeStageVolume: could not resize volume %v (%v):  %v", diskSource, stagingTargetPath, err)
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (node *cbsNode) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	glog.Infof("NodeUnstageVolume: start with args %v", *req)

	if req.StagingTargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "volume staging target path is empty")
	}
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume id is empty")
	}

	stagingTargetPath := req.StagingTargetPath

	_, refCount, err := mount.GetDeviceNameFromMount(node.mounter, stagingTargetPath)
	fmt.Printf("refCount is %v", refCount)
	if err != nil {
		glog.Errorf("NodeUnstageVolume: GetDeviceNameFromMount error %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	if refCount == 0 {
		glog.Infof("NodeUnstageVolume: %v is not mounted", stagingTargetPath)
		return &csi.NodeUnstageVolumeResponse{}, nil
	}

	if err := node.mounter.Unmount(stagingTargetPath); err != nil {
		glog.Errorf("NodeUnstageVolume: Unmount %v error %v", stagingTargetPath, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (node *cbsNode) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	glog.Infof("NodePublishVolume: start with args %v", *req)

	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume id is empty")
	}
	if req.StagingTargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "volume staging target path is empty")
	}
	if req.TargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "volume target path is empty")
	}
	if req.VolumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "volume has no capabilities")
	}

	if req.VolumeCapability.GetMount() == nil {
		return nil, status.Error(codes.InvalidArgument, "volume access type is not mount")
	}

	source := req.StagingTargetPath
	target := req.TargetPath

	mountFlags := req.VolumeCapability.GetMount().MountFlags
	mountFlags = append(mountFlags, "bind")

	if req.Readonly {
		mountFlags = append(mountFlags, "ro")
	}

	mountFsType := req.VolumeCapability.GetMount().FsType

	if mountFsType == "" {
		mountFsType = "ext4"
	}

	if err := node.checkStagingTargetPath(ctx, req); err != nil {
		return nil, err
	}

	notMnt, err := node.mounter.IsLikelyNotMountPoint(target)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(target, 0750); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			notMnt = true
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if !notMnt {
		glog.Infof("NodePublishVolume: TargetPath %s is already mounted, skipping", target)
		return &csi.NodePublishVolumeResponse{}, nil
	}

	if err := node.mounter.Mount(source, target, mountFsType, mountFlags); err != nil {
		glog.Errorf("NodePublishVolume: Mount error target %v error %v", target, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (node *cbsNode) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	glog.Infof("NodeUnpublishVolume: start with args %v", *req)

	if req.TargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "volume target path is empty")
	}

	targetPath := req.TargetPath

	if err := node.cleanIntreePath(targetPath, req.VolumeId); err != nil {
		glog.Errorf("NodeUnpublishVolume: cleanIntreePath failed, error %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	for n := 0; ; n++ {
		notMnt, err := node.mounter.IsLikelyNotMountPoint(targetPath)
		if err != nil {
			if os.IsNotExist(err) {
				glog.Infof("NodeUnpublishVolume: targetPath %v is already deleted", targetPath)
				return &csi.NodeUnpublishVolumeResponse{}, nil
			} else {
				return nil, status.Error(codes.Internal, err.Error())
			}
		}
		if notMnt {
			if n == 0 {
				glog.Infof("NodeUnpublishVolume: targetPath %v is already unmounted", targetPath)
				return &csi.NodeUnpublishVolumeResponse{}, nil
			}
			glog.Infof("NodeUnpublishVolume: umount targetPath %v for %d times", targetPath, n)
			break
		}

		if err := node.mounter.Unmount(targetPath); err != nil {
			glog.Errorf("NodeUnpublishVolume: Unmount targetPath %v failed, error %v", targetPath, err)
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (node *cbsNode) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
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

func (node *cbsNode) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{
		NodeId:            node.nodeID,
		MaxVolumesPerNode: node.getMaxAttachVolumePerNode(),

		// make sure that the driver works on this particular zone only
		AccessibleTopology: &csi.Topology{
			Segments: map[string]string{
				TopologyZoneKey: node.zone,
			},
		},
	}, nil
}

func (node *cbsNode) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	glog.Infof("NodeGetVolumeStats: NodeGetVolumeStatsRequest is %v", *req)

	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}
	VolumePath := req.GetVolumePath()
	if len(VolumePath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume path is empty")
	}

	volumeMetrics, err := volume.NewMetricsStatFS(req.VolumePath).GetMetrics()
	if err != nil {
		return nil, err
	}

	available, ok := volumeMetrics.Available.AsInt64()
	if !ok {
		return nil, status.Errorf(codes.Internal, "Volume metrics available %v is invalid", volumeMetrics.Available)
	}
	capacity, ok := volumeMetrics.Capacity.AsInt64()
	if !ok {
		return nil, status.Errorf(codes.Internal, "Volume metrics capacity %v is invalid", volumeMetrics.Capacity)
	}
	used, ok := volumeMetrics.Used.AsInt64()
	if !ok {
		return nil, status.Errorf(codes.Internal, "Volume metrics used %v is invalid", volumeMetrics.Used)
	}

	inodesFree, ok := volumeMetrics.InodesFree.AsInt64()
	if !ok {
		return nil, status.Errorf(codes.Internal, "Volume metrics inodesFree %v is invalid", volumeMetrics.InodesFree)
	}
	inodes, ok := volumeMetrics.Inodes.AsInt64()
	if !ok {
		return nil, status.Errorf(codes.Internal, "Volume metrics inodes %v is invalid", volumeMetrics.Inodes)
	}
	inodesUsed, ok := volumeMetrics.InodesUsed.AsInt64()
	if !ok {
		return nil, status.Errorf(codes.Internal, "Volume metrics inodesUsed %v is invalid", volumeMetrics.InodesUsed)
	}

	return &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{
				Available: available,
				Total:     capacity,
				Used:      used,
				Unit:      csi.VolumeUsage_BYTES,
			},
			{
				Available: inodesFree,
				Total:     inodes,
				Used:      inodesUsed,
				Unit:      csi.VolumeUsage_INODES,
			},
		},
	}, nil
}

func (node *cbsNode) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	glog.Infof("NodeExpandVolume: NodeExpandVolumeRequest is %v", *req)

	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}
	volumePath := req.GetVolumePath()
	if volumePath == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume Path not provided")
	}

	args := []string{"-o", "source", "--noheadings", "--target", volumePath}
	output, err := node.mounter.Exec.Command("findmnt", args...).Output()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not determine device path: %v, raw block device or unmounted", err)
	}

	devicePath := strings.TrimSpace(string(output))
	if len(devicePath) == 0 {
		return nil, status.Errorf(codes.Internal, "Could not get valid device for mount path: %v", volumePath)
	}

	err = checkVolumePathCapacity(devicePath, req.CapacityRange.RequiredBytes)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "check volumePath(%s) capacity failed, error: %v", volumePath, err)
	}

	r := resizefs.NewResizeFs(&node.mounter)
	if _, err := r.Resize(devicePath, volumePath); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not resize volume %s %s: %v", volumeID, devicePath, err)
	}

	return &csi.NodeExpandVolumeResponse{}, nil
}

func (node *cbsNode) checkStagingTargetPath(ctx context.Context, req *csi.NodePublishVolumeRequest) error {
	stagingTargetPath := req.GetStagingTargetPath()

	notMnt, err := node.mounter.IsLikelyNotMountPoint(stagingTargetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(stagingTargetPath, 0750); err != nil {
				return status.Error(codes.Internal, err.Error())
			}
			notMnt = true
		} else {
			return status.Error(codes.Internal, err.Error())
		}
	}

	if notMnt {
		glog.Infof("Call NodeStageVolume to prepare the stagingTargetPath %s", stagingTargetPath)
		stageReq := &csi.NodeStageVolumeRequest{
			VolumeId:          req.GetVolumeId(),
			VolumeCapability:  req.GetVolumeCapability(),
			StagingTargetPath: req.GetStagingTargetPath(),
		}
		_, err = node.NodeStageVolume(ctx, stageReq)
		if err != nil {
			return err
		}
	}

	return nil
}

func (node *cbsNode) cleanIntreePath(targetPath, volumeId string) error {
	if err := node.cleanIntreeTargetPath(targetPath); err != nil {
		return err
	}

	if err := node.cleanIntreeStagingPath(targetPath, volumeId); err != nil {
		return err
	}

	return nil
}

func (node *cbsNode) cleanIntreeTargetPath(targetPath string) error {
	intreeTargetPath := convertToIntreeTargetPath(targetPath)

	if pathExists, err := util.PathExists(intreeTargetPath); err != nil {
		return err
	} else if !pathExists {
		return nil
	}

	glog.Infof("try to clean intreeTargetPath %s", intreeTargetPath)
	if err := mount.CleanupMountPoint(intreeTargetPath, node.mounter, false); err != nil {
		return err
	}

	return nil
}

func (node *cbsNode) cleanIntreeStagingPath(targetPath, volumeId string) error {
	intreeStagingPath := convertToIntreeStagingPath(targetPath, volumeId)

	if pathExists, err := util.PathExists(intreeStagingPath); err != nil {
		return err
	} else if !pathExists {
		return nil
	}

	glog.Infof("try to clean intreeStagingPath %s", intreeStagingPath)
	if err := mount.CleanupMountPoint(intreeStagingPath, node.mounter, false); err != nil {
		return err
	}

	return nil
}

func (node *cbsNode) getMaxAttachVolumePerNode() int64 {
	if node.volumeAttachLimit >= 0 {
		return node.volumeAttachLimit
	}

	return int64(defaultMaxAttachVolumePerNode)
}

func isNotSupport(err error) bool {
	if strings.Contains(err.Error(), "resize of format") && strings.Contains(err.Error(), "is not supported for device") {
		return true
	}
	return false
}

func getInstanceIdFromProviderID(client kubernetes.Interface) string {
	nodeName := os.Getenv(NodeNameKey)
	providerID := getProviderIDFromNode(client, nodeName)
	u, err := url.Parse(providerID)
	if err != nil {
		glog.Errorf("parse the providerID %s in node %s failed, err: %v", providerID, nodeName, err)
		return ""
	}

	switch u.Scheme {
	case "qcloud":
		tokens := strings.Split(strings.Trim(u.Path, "/"), "/")
		instanceId := tokens[len(tokens)-1]
		if instanceId == "" || strings.Contains(instanceId, "/") || !strings.HasPrefix(instanceId, "ins-") {
			glog.Errorf("invalid format for qcloud instance (%s)", providerID)
			return ""
		}
		return instanceId
	case "tencentcloud":
		if u.Host == "" || strings.Contains(u.Host, "/") || !strings.HasPrefix(u.Host, "kn-") {
			glog.Errorf("invalid format for tencentcloud instance (%s)", providerID)
			return ""
		}
		return strings.ReplaceAll(u.Host, "kn-", "eks-")
	default:
		glog.Errorf("not support providerID %s in node %s", providerID, nodeName)
		return ""
	}
}

func getProviderIDFromNode(client kubernetes.Interface, nodeName string) string {
	ctx := context.Background()
	ticker := time.NewTicker(time.Second * 2)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()

	for {
		select {
		case <-ticker.C:
			node, err := client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
			if err != nil {
				glog.Errorf("get node %s failed, err: %v", nodeName, err)
				continue
			}
			if node.Spec.ProviderID == "" {
				glog.Errorf("the providerID in node %s is empty", nodeName)
				continue
			}
			return node.Spec.ProviderID
		case <-ctx.Done():
			glog.Fatalf("get providerID from node %s failed before deadline exceeded", nodeName)
		}
	}
}

func findCBSVolume(diskId string) (device string, err error) {
	p := filepath.Join(DiskByIDDevicePath, DiskByIDDeviceNamePrefix+diskId)

	stat, err := os.Lstat(p)
	if err != nil {
		if os.IsNotExist(err) {
			metrics.DevicePathNotExist.WithLabelValues(DriverName).Inc()
			glog.Warningf("cbs block path %s not found. We will get device from serial(/sys/block/vdX/serail)", p)
			deviceFromSerial, err := getDevicePathsBySerial(diskId)
			if err != nil {
				return "", err
			}

			if err := os.Symlink(deviceFromSerial, p); err != nil {
				glog.Errorf("Failed to link devicePathFromSerial(%s) and devicePathFromKubelet(%s): %v", deviceFromSerial, p, err)
				return "", err
			}

			glog.Infof("Successfully get device(%s) from serial(/sys/block/vdX/serail), and Symlink %s and %s", deviceFromSerial, deviceFromSerial, p)
			return deviceFromSerial, nil
		}
		return "", fmt.Errorf("error getting stat of %q: %v", p, err)
	}

	if stat.Mode()&os.ModeSymlink != os.ModeSymlink {
		glog.Warningf("cbs block file %q found, but was not a symlink", p)
		return "", fmt.Errorf("cbs block file %q found, but was not a symlink", p)
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", fmt.Errorf("error reading target of symlink %q: %v", p, err)
	}

	if !strings.HasPrefix(resolved, "/dev") {
		return "", fmt.Errorf("resolved symlink for %q was unexpected: %q", p, resolved)
	}

	return resolved, nil
}

func getDevicePathsBySerial(diskId string) (string, error) {
	dirs, _ := filepath.Glob("/sys/block/*")
	for _, dir := range dirs {
		serialPath := filepath.Join(dir, "serial")
		serialPathExist, err := pathExist(serialPath)
		if err != nil {
			return "", err
		}

		if serialPathExist {
			content, err := ioutil.ReadFile(serialPath)
			if err != nil {
				glog.Errorf("Failed to get diskId from serial path(%s): %v", serialPath, err)
				return "", err
			}

			if strings.Trim(string(content), " ") == diskId {
				arr := strings.Split(dir, "/")
				return filepath.Join("/dev/", arr[len(arr)-1]), nil
			}
		}
	}

	glog.Errorf("can not find diskId %v by serial", diskId)
	return "", fmt.Errorf("can not find diskId %v by serial", diskId)
}

func pathExist(path string) (bool, error) {
	_, err := os.Stat(path)

	if err == nil {
		return true, nil
	} else if os.IsNotExist(err) {
		return false, nil
	} else {
		return false, err
	}
}

func checkVolumePathCapacity(devicePath string, requiredBytes int64) error {
	file, err := os.Open(devicePath)
	if err != nil {
		return fmt.Errorf("open devicePath %s failed, err: %s", devicePath, err)
	}
	capacity, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("seek to end of device %s failed, err: %s", devicePath, err)
	}
	if capacity != requiredBytes {
		return fmt.Errorf("device haven't resized, device: %v, required: %v", capacity, requiredBytes)
	}
	return nil
}

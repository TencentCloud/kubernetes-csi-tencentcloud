package cbs

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
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
	cleanDevicemapper bool
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
		cleanDevicemapper: drv.cleanDevicemapper,
	}
}

func (node *cbsNode) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume id is empty")
	}
	if req.StagingTargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "volume staging target path is empty")
	}
	if req.VolumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "volume has no capabilities")
	}

	// 1. check if current req is in progress.
	if ok := node.idempotent.Insert(req); !ok {
		return nil, status.Errorf(codes.Internal, "volume %v is in progress", req.VolumeId)
	}

	defer func() {
		glog.Infof("NodeStageVolume: volume %v finished", req.VolumeId)
		node.idempotent.Delete(req)
	}()

	diskID := req.VolumeId
	stagingTargetPath := req.StagingTargetPath
	mountFlags := req.GetVolumeCapability().GetMount().GetMountFlags()

	isBlock := req.GetVolumeCapability().GetBlock() != nil
	if isBlock {
		stagingTargetPath = filepath.Join(stagingTargetPath, req.VolumeId)
	}

	err := createMountPoint(stagingTargetPath, isBlock)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "createMountPoint failed, err: %v", err)
	}

	//2. check target path mounted
	var diskSource string
	devicemapper := ""
	if v, ok := req.VolumeContext["devicemapper"]; ok {
		devicemapper = v
	}

	switch devicemapper {
	case "RAID":
		diskSource, err = mdadmCreate(diskID, req)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "mdadmCreate failed, err: %v", err)
		}
	case "LVM":
		diskSource, err = lvmCreate(diskID, req)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "lvmCreate failed, err: %v", err)
		}
	case "CRYPT":
		//TODO
	default:
		diskSource, err = findCBSVolume(diskID)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "findCBSVolume error cbs disk=%v, error %v", filepath.Join(DiskByIDDevicePath, DiskByIDDeviceNamePrefix+diskID), err)
		}

		if isBlock {
			notMnt, err := node.mounter.IsLikelyNotMountPoint(stagingTargetPath)
			if err != nil {
				if !os.IsNotExist(err) {
					return nil, status.Error(codes.NotFound, err.Error())
				}
			}
			if notMnt {
				mountFlags = append(mountFlags, "bind")
				err = node.mounter.Mount(diskSource, stagingTargetPath, "", mountFlags)
				if err != nil {
					return nil, status.Errorf(codes.Internal, "Mount failed, diskSource: %s, stagingTargetPath: %s, err: %v", diskSource, stagingTargetPath, err)
				}
			}
			return &csi.NodeStageVolumeResponse{}, nil
		}

		device, _, err := mount.GetDeviceNameFromMount(node.mounter, stagingTargetPath)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "GetDeviceNameFromMount failed, err: %v", err)
		}
		if diskSource == device || filepath.Join(DiskByIDDevicePath, DiskByIDDeviceNamePrefix+diskID) == device {
			glog.Infof("NodeStageVolume: volume %s is already staged", diskID)
			return &csi.NodeStageVolumeResponse{}, nil
		}
	}

	mountFsType := req.GetVolumeCapability().GetMount().GetFsType()
	if mountFsType == "xfs" {
		mountFlags = append(mountFlags, "nouuid")
	}

	if err := node.mounter.FormatAndMount(diskSource, stagingTargetPath, mountFsType, mountFlags); err != nil {
		return nil, status.Errorf(codes.Internal, "FormatAndMount failed, diskSource: %s, stagingTargetPath: %s, err: %v", diskSource, stagingTargetPath, err)
	}

	r := resizefs.NewResizeFs(&node.mounter)
	if _, err := r.Resize(diskSource, stagingTargetPath); err != nil && !isNotSupport(err) {
		return nil, status.Errorf(codes.Internal, "NodeStageVolume: Resize failed, diskSource: %s, stagingTargetPath: %s, err: %v", diskSource, stagingTargetPath, err)
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (node *cbsNode) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	if req.StagingTargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "volume staging target path is empty")
	}
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume id is empty")
	}

	diskIds := req.VolumeId
	stagingTargetPath := req.StagingTargetPath

	tmpPath := filepath.Join(req.StagingTargetPath, diskIds)
	notMnt, err := node.mounter.IsLikelyNotMountPoint(tmpPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		notMnt = true
	}
	if !notMnt {
		if err = node.mounter.Unmount(tmpPath); err != nil {
			return nil, status.Errorf(codes.Internal, "Unmount %s failed, err: %v", tmpPath, err)
		}
		if err = os.Remove(tmpPath); err != nil {
			if !os.IsNotExist(err) {
				return nil, status.Errorf(codes.Internal, "Remove %s failed, err: %v", tmpPath, err)
			}
		}
		return &csi.NodeUnstageVolumeResponse{}, nil
	}

	_, refCount, err := mount.GetDeviceNameFromMount(node.mounter, stagingTargetPath)
	fmt.Printf("refCount is %v", refCount)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "GetDeviceNameFromMount %s failed, err: %v", stagingTargetPath, err)
	}
	if refCount == 0 {
		glog.Infof("NodeUnstageVolume: %v is not mounted", stagingTargetPath)
		return &csi.NodeUnstageVolumeResponse{}, nil
	}

	if err := node.mounter.Unmount(stagingTargetPath); err != nil {
		return nil, status.Errorf(codes.Internal, "Unmount %s failed, err: %v", stagingTargetPath, err)
	}

	if strings.Contains(diskIds, ",") {
		if !node.cleanDevicemapper {
			glog.Infof("NodeUnstageVolume: skip devicemapper clean for volume %s", diskIds)
			return &csi.NodeUnstageVolumeResponse{}, nil
		}

		// TODO, we hard code lvm here to support lvm only, and add a flag in the volumeId such as 'lvm,disk-xxx,disk-yyy' to support raid.
		deviceMapper := "LVM"
		glog.Infof("devicemapper raid/lvm enabled, should clean raid/lvm config")
		switch deviceMapper {
		case "RAID":
			err := mdadmDelete(diskIds, stagingTargetPath)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "mdadmDelete failed, err: %v", err)
			}
		case "LVM":
			err = lvmDelete(stagingTargetPath)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "lvmDelete failed, err: %v", err)
			}
		}
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

	isBlock := req.GetVolumeCapability().GetBlock() != nil

	target := req.TargetPath
	source := req.StagingTargetPath
	if isBlock {
		source = filepath.Join(req.StagingTargetPath, req.VolumeId)
	}

	mountFlags := req.GetVolumeCapability().GetMount().GetMountFlags()
	mountFlags = append(mountFlags, "bind")
	if req.Readonly {
		mountFlags = append(mountFlags, "ro")
	}

	mountFsType := req.GetVolumeCapability().GetMount().GetFsType()
	if mountFsType == "" {
		mountFsType = "ext4"
	}

	if !isBlock {
		if err := node.checkStagingTargetPath(ctx, req); err != nil {
			return nil, err
		}
	}

	notMnt, err := node.mounter.IsLikelyNotMountPoint(target)
	if err == nil && !notMnt {
		glog.Infof("NodePublishVolume: TargetPath %s is already mounted, skipping", target)
		return &csi.NodePublishVolumeResponse{}, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, status.Error(codes.Internal, err.Error())
	}

	err = createMountPoint(target, isBlock)
	if err != nil {
		glog.Errorf("NodeStageVolume: createMountPoint error %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	if isBlock {
		err = node.mounter.Mount(source, target, "", mountFlags)
	} else {
		err = node.mounter.Mount(source, target, mountFsType, mountFlags)
	}
	if err != nil {
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
	for _, nodeCap := range nodeCaps {
		c := &csi.NodeServiceCapability{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: nodeCap,
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
				TopologyZoneKey:     node.zone,
				TopologyInstanceKey: node.nodeID,
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
	diskIds := req.GetVolumeId()
	if diskIds == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID not provided")
	}
	volumePath := req.GetVolumePath()
	if volumePath == "" {
		return nil, status.Error(codes.InvalidArgument, "volume Path not provided")
	}

	if strings.Contains(volumePath, "volumeDevices") {
		glog.Infof("NodeExpandVolume: skip block volume")
		return &csi.NodeExpandVolumeResponse{}, nil
	}

	args := []string{"-o", "source", "--noheadings", "--target", volumePath}
	output, err := node.mounter.Exec.Command("findmnt", args...).Output()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not determine device path: %v, raw block device or unmounted", err)
	}

	devicePath := strings.TrimSpace(string(output))
	if len(devicePath) == 0 {
		return nil, status.Errorf(codes.Internal, "could not get valid device for mount path: %v", volumePath)
	}

	if strings.Contains(diskIds, ",") {
		err = lvmExpand(diskIds, devicePath, req.CapacityRange.RequiredBytes)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	} else {
		err = checkVolumePathCapacity(devicePath, req.CapacityRange.RequiredBytes)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "check volumePath(%s) capacity failed, err: %v", volumePath, err)
		}
	}

	r := resizefs.NewResizeFs(&node.mounter)
	if _, err := r.Resize(devicePath, volumePath); err != nil {
		return nil, status.Errorf(codes.Internal, "NodeExpandVolume: Resize failed, devicePath %s, volumePath %s, err: %v", diskIds, devicePath, err)
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
		volumeContext := make(map[string]string)
		if strings.Contains(req.GetVolumeId(), ",") {
			devices := len(strings.Split(req.GetVolumeId(), ","))
			volumeContext["devicemapper"] = "LVM"
			volumeContext["devices"] = strconv.Itoa(devices)
		}
		stageReq := &csi.NodeStageVolumeRequest{
			VolumeId:          req.GetVolumeId(),
			VolumeCapability:  req.GetVolumeCapability(),
			StagingTargetPath: req.GetStagingTargetPath(),
			VolumeContext:     volumeContext,
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
	if intreeTargetPath == "" {
		return nil
	}

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
	if intreeStagingPath == "" {
		return nil
	}

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

func createMountPoint(mountPath string, isBlock bool) error {
	fi, err := os.Stat(mountPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if isBlock {
		if err == nil && fi.IsDir() {
			os.Remove(mountPath)
		}
		pathFile, err := os.OpenFile(mountPath, os.O_CREATE|os.O_RDWR, 0750)
		if err != nil {
			return err
		}
		if err = pathFile.Close(); err != nil {
			return err
		}
		return nil
	}

	if os.IsNotExist(err) {
		err := os.MkdirAll(mountPath, 0750)
		if err != nil {
			return err
		}
	}

	return nil
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
				return "", fmt.Errorf("Failed to link devicePathFromSerial(%s) and devicePathFromKubelet(%s): %v ", deviceFromSerial, p, err)
			}

			glog.Infof("Successfully get device(%s) from serial(/sys/block/vdX/serail), and Symlink %s and %s", deviceFromSerial, deviceFromSerial, p)
			return deviceFromSerial, nil
		}
		return "", fmt.Errorf("error getting stat of %q: %v", p, err)
	}

	if stat.Mode()&os.ModeSymlink != os.ModeSymlink {
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
				return "", fmt.Errorf("Failed to get diskId from serial path(%s): %v ", serialPath, err)
			}

			if strings.Trim(string(content), " ") == diskId {
				arr := strings.Split(dir, "/")
				return filepath.Join("/dev/", arr[len(arr)-1]), nil
			}
		}
	}

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
	if capacity < requiredBytes {
		return fmt.Errorf("device haven't resized, device: %v, required: %v", capacity, requiredBytes)
	}
	return nil
}

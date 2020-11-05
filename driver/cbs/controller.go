package cbs

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	cbs "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cbs/v20170312"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"

	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/util"
)

var (
	GB uint64 = 1 << (10 * 3)

	DiskTypeCloudBasic   = "CLOUD_BASIC"
	DiskTypeCloudPremium = "CLOUD_PREMIUM"
	DiskTypeCloudSsd     = "CLOUD_SSD"

	DiskTypeDefault = DiskTypeCloudPremium

	// cbs disk charge type
	DiskChargeTypePrePaid        = "PREPAID"
	DiskChargeTypePostPaidByHour = "POSTPAID_BY_HOUR"

	DiskChargeTypeDefault = DiskChargeTypePostPaidByHour

	// cbs disk charge prepaid options
	DiskChargePrepaidPeriodAttr = "diskChargeTypePrepaidPeriod"

	DiskChargePrepaidPeriodValidValues = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 24, 36}
	DiskChargePrepaidPeriodDefault     = 1

	DiskChargePrepaidRenewFlagNotifyAndAutoRenew          = "NOTIFY_AND_AUTO_RENEW"
	DiskChargePrepaidRenewFlagNotifyAndManualRenewd       = "NOTIFY_AND_MANUAL_RENEW"
	DiskChargePrepaidRenewFlagDisableNotifyAndManualRenew = "DISABLE_NOTIFY_AND_MANUAL_RENEW"

	DiskChargePrepaidRenewFlagDefault = DiskChargePrepaidRenewFlagNotifyAndManualRenewd

	// cbs disk encrypt
	EncryptAttr   = "encrypt"
	EncryptEnable = "ENCRYPT"

	//cbs disk zone
	DiskZone = "diskZone"

	//cbs disk zones
	DiskZones = "diskZones"

	TagForDeletionCreateBy  = "tke-cbs-provisioner-createBy-flag"
	TagForDeletionClusterId = "tke-clusterId"

	// cbs status
	StatusUnattached = "UNATTACHED"
	StatusAttached   = "ATTACHED"
	StatusExpanding  = "EXPANDING"

	// volumeCaps represents how the volume could be accessed.
	// It is SINGLE_NODE_WRITER since EBS volume could only be
	// attached to a single node at any given time.
	volumeCaps = []csi.VolumeCapability_AccessMode{
		{
			Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		},
	}

	// controllerCaps represents the capability of controller service
	controllerCaps = []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
	}

	SnapshotNormal = "NORMAL"

	cbsSnapshotsMapsCache = &cbsSnapshotsCache{
		mux:             &sync.Mutex{},
		cbsSnapshotMaps: make(map[string]*cbsSnapshot),
	}
)

type cbsController struct {
	cbsClient     *cbs.Client
	zone          string
	clusterId     string
	metadataStore util.CachePersister
}

func newCbsController(region, zone, cbsUrl, clusterId string, cachePersister util.CachePersister) (*cbsController, error) {
	secretID, secretKey, token, _ := util.GetSercet()
	cred := &common.Credential{
		SecretId:  secretID,
		SecretKey: secretKey,
		Token:     token,
	}

	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = cbsUrl
	client, err := cbs.NewClient(cred, region, cpf)
	if err != nil {
		return nil, err
	}

	return &cbsController{
		cbsClient:     client,
		zone:          zone,
		clusterId:     clusterId,
		metadataStore: cachePersister,
	}, nil
}

func (ctrl *cbsController) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "volume name is empty")
	}

	volumeIdempotencyName := req.Name
	volumeCapacity := req.CapacityRange.RequiredBytes

	if len(req.VolumeCapabilities) <= 0 {
		return nil, status.Error(codes.InvalidArgument, "volume has no capabilities")
	}

	for _, c := range req.VolumeCapabilities {
		if c.GetBlock() != nil {
			return nil, status.Error(codes.InvalidArgument, "block volume is not supported")
		}
		if c.AccessMode.Mode != csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER {
			return nil, status.Error(codes.InvalidArgument, "block access mode only support singer node writer")
		}
	}

	var aspId, volumeZone string
	volumeType := DiskTypeDefault
	volumeChargeType := DiskChargeTypeDefault
	volumeChargePrepaidRenewFlag := DiskChargePrepaidRenewFlagDefault
	volumeChargePrepaidPeriod := 1
	projectId := 0
	volumeTags := make([]*cbs.Tag, 0)
	for k, v := range req.Parameters {
		switch strings.ToLower(k) {
		case "aspid":
			aspId = v
		case "type", "disktype":
			volumeType = v
		case "zone", "diskzone":
			volumeZone = v
		case "paymode", "diskchargetype":
			volumeChargeType = v
		case "renewflag", "diskchargeprepaidrenewflag":
			volumeChargePrepaidRenewFlag = v
		case "diskchargetypeprepaidperiod":
			var err error
			volumeChargePrepaidPeriod, err = strconv.Atoi(v)
			if err != nil {
				glog.Infof("volumeChargePrepaidPeriod atoi error: %v", err)
			}
		case "project":
			var err error
			projectId, err = strconv.Atoi(v)
			if err != nil {
				glog.Infof("projectId atoi error: %v", err)
			}
		case "disktags":
			tags := strings.Split(v, ",")
			for _, tag := range tags {
				kv := strings.Split(tag, ":")
				if kv == nil || len(kv) != 2 {
					continue
				}
				volumeTags = append(volumeTags, &cbs.Tag{
					Key:   &kv[0],
					Value: &kv[1],
				})
			}
		default:
		}
	}

	if volumeType != DiskTypeCloudBasic && volumeType != DiskTypeCloudPremium && volumeType != DiskTypeCloudSsd {
		return nil, status.Error(codes.InvalidArgument, "cbs type not supported")
	}

	if volumeChargeType == DiskChargeTypePrePaid {
		found := false
		for _, p := range DiskChargePrepaidPeriodValidValues {
			if p == volumeChargePrepaidPeriod {
				found = true
			}
		}

		if !found {
			return nil, status.Error(codes.InvalidArgument, "can not found valid prepaid period")
		}

		if volumeChargePrepaidRenewFlag != DiskChargePrepaidRenewFlagDisableNotifyAndManualRenew && volumeChargePrepaidRenewFlag != DiskChargePrepaidRenewFlagNotifyAndAutoRenew && volumeChargePrepaidRenewFlag != DiskChargePrepaidRenewFlagNotifyAndManualRenewd {
			return nil, status.Error(codes.InvalidArgument, "invalid renew flag")
		}

	}

	volumeEncrypt, ok := req.Parameters[EncryptAttr]
	if !ok {
		volumeEncrypt = ""
	}

	if volumeEncrypt != "" && volumeEncrypt != EncryptEnable {
		return nil, status.Error(codes.InvalidArgument, "volume encrypt not valid")
	}

	createCbsReq := cbs.NewCreateDisksRequest()

	diskName := volumeIdempotencyName
	if ctrl.clusterId != "" {
		diskName = fmt.Sprintf("%s/%s", ctrl.clusterId, volumeIdempotencyName)
	}
	createCbsReq.DiskName = common.StringPtr(diskName)
	createCbsReq.ClientToken = &volumeIdempotencyName
	createCbsReq.DiskType = &volumeType
	createCbsReq.DiskChargeType = &volumeChargeType

	if volumeChargeType == DiskChargeTypePrePaid {
		period := uint64(volumeChargePrepaidPeriod)
		createCbsReq.DiskChargePrepaid = &cbs.DiskChargePrepaid{
			Period:    &period,
			RenewFlag: &volumeChargePrepaidRenewFlag,
		}
	}

	gb := uint64(volumeCapacity / int64(GB))

	createCbsReq.DiskSize = &gb

	if volumeEncrypt == EncryptEnable {
		createCbsReq.Encrypt = &EncryptEnable
	}

	//zone parameters
	// volumeZone, ok := req.Parameters[DiskZone]
	// volumeZones, ok2 := req.Parameters[DiskZones]
	// if ok1 && ok2 {
	// 	return nil, status.Error(codes.InvalidArgument, "both zone and zones StorageClass parameters must not be used at the same time")
	// }
	glog.Infof("req.GetAccessibilityRequirements() is %v", req.GetAccessibilityRequirements())
	if !ok {
		volumeZone = pickAvailabilityZone(req.GetAccessibilityRequirements())
	}
	// TODO maybe we don't need this, controller plugin' node zone is not a property zone for pod.
	if volumeZone == "" {
		volumeZone = ctrl.zone
	}

	createCbsReq.Placement = &cbs.Placement{
		Zone:      &volumeZone,
		ProjectId: common.Uint64Ptr(uint64(projectId)),
	}

	updateCbsClent(ctrl.cbsClient)
	snapshotId := ""
	if req.VolumeContentSource != nil {
		snapshot := req.VolumeContentSource.GetSnapshot()
		if snapshot == nil {
			return nil, status.Error(codes.InvalidArgument, "Volume Snapshot cannot be empty")
		}
		snapshotId = snapshot.GetSnapshotId()
		if len(snapshotId) == 0 {
			return nil, status.Error(codes.InvalidArgument, "Volume Snapshot ID cannot be empty")
		}
		listSnapshotRequest := cbs.NewDescribeSnapshotsRequest()
		listSnapshotRequest.SnapshotIds = []*string{&snapshotId}

		listSnapshotResponse, err := ctrl.cbsClient.DescribeSnapshots(listSnapshotRequest)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		if len(listSnapshotResponse.Response.SnapshotSet) <= 0 {
			return nil, status.Error(codes.NotFound, "Volume Snapshot not found")
		}
		createCbsReq.SnapshotId = &snapshotId
	}
	// must add VolumeContentSource for snapshot volume
	var src *csi.VolumeContentSource
	if snapshotId != "" {
		src = &csi.VolumeContentSource{
			Type: &csi.VolumeContentSource_Snapshot{
				Snapshot: &csi.VolumeContentSource_SnapshotSource{
					SnapshotId: snapshotId,
				},
			},
		}
	}

	createCbsReq.Tags = append(createCbsReq.Tags, volumeTags...)

	if ctrl.clusterId != "" {
		createCbsReq.Tags = append(createCbsReq.Tags, &cbs.Tag{Key: common.StringPtr(TagForDeletionCreateBy), Value: common.StringPtr("yes")})
		createCbsReq.Tags = append(createCbsReq.Tags, &cbs.Tag{Key: common.StringPtr(TagForDeletionClusterId), Value: common.StringPtr(ctrl.clusterId)})
	}

	glog.Infof("createCbsReq: %+v", createCbsReq)

	createCbsResponse, err := ctrl.cbsClient.CreateDisks(createCbsReq)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if len(createCbsResponse.Response.DiskIdSet) <= 0 {
		return nil, status.Errorf(codes.Internal, "create disk failed, no disk id found in create disk response, request id %s", *createCbsResponse.Response.RequestId)
	}

	diskId := *createCbsResponse.Response.DiskIdSet[0]

	disk := new(cbs.Disk)

	ticker := time.NewTicker(time.Second * 5)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*120)
	defer cancel()

	for {
		select {
		case <-ticker.C:
			listCbsRequest := cbs.NewDescribeDisksRequest()
			listCbsRequest.DiskIds = []*string{&diskId}

			listCbsResponse, err := ctrl.cbsClient.DescribeDisks(listCbsRequest)
			if err != nil {
				continue
			}
			if len(listCbsResponse.Response.DiskSet) >= 1 {
				for _, d := range listCbsResponse.Response.DiskSet {
					if *d.DiskId == diskId && d.DiskState != nil {
						if *d.DiskState == StatusAttached || *d.DiskState == StatusUnattached {
							disk = d
							if aspId != "" {
								bindReq := cbs.NewBindAutoSnapshotPolicyRequest()
								bindReq.AutoSnapshotPolicyId = &aspId
								bindReq.DiskIds = []*string{disk.DiskId}
								_, err := ctrl.cbsClient.BindAutoSnapshotPolicy(bindReq)
								if err != nil {

								}
							}
							return &csi.CreateVolumeResponse{
								Volume: &csi.Volume{
									CapacityBytes: int64(*disk.DiskSize * GB),
									ContentSource: src,
									VolumeId:      *disk.DiskId,
									VolumeContext: req.GetParameters(),
									// TODO verify this topology
									AccessibleTopology: []*csi.Topology{
										{
											Segments: map[string]string{
												TopologyZoneKey: volumeZone,
											},
										},
									},
								},
							}, nil
						}
					}
				}
			}
		case <-ctx.Done():
			return nil, status.Error(codes.DeadlineExceeded, fmt.Sprintf("cbs disk is not ready before deadline exceeded %s", diskId))
		}
	}
}

func (ctrl *cbsController) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume id is empty")
	}

	describeDiskRequest := cbs.NewDescribeDisksRequest()
	describeDiskRequest.DiskIds = []*string{&req.VolumeId}
	updateCbsClent(ctrl.cbsClient)
	describeDiskResponse, err := ctrl.cbsClient.DescribeDisks(describeDiskRequest)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if len(describeDiskResponse.Response.DiskSet) <= 0 {
		return &csi.DeleteVolumeResponse{}, nil
	}

	terminateCbsRequest := cbs.NewTerminateDisksRequest()
	terminateCbsRequest.DiskIds = []*string{&req.VolumeId}

	_, err = ctrl.cbsClient.TerminateDisks(terminateCbsRequest)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.DeleteVolumeResponse{}, nil
}

func (ctrl *cbsController) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume id is empty")
	}
	if req.NodeId == "" {
		return nil, status.Error(codes.InvalidArgument, "node id is empty")
	}

	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "volume has no capabilities")
	}

	diskId := req.VolumeId
	instanceId := req.NodeId

	listCbsRequest := cbs.NewDescribeDisksRequest()
	listCbsRequest.DiskIds = []*string{&diskId}
	updateCbsClent(ctrl.cbsClient)
	listCbsResponse, err := ctrl.cbsClient.DescribeDisks(listCbsRequest)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if len(listCbsResponse.Response.DiskSet) <= 0 {
		return nil, status.Error(codes.NotFound, "disk not found")
	}

	for _, disk := range listCbsResponse.Response.DiskSet {
		if *disk.DiskId == diskId {
			if *disk.DiskState == StatusAttached && *disk.InstanceId == instanceId {
				return &csi.ControllerPublishVolumeResponse{}, nil
			}
			if *disk.DiskState == StatusAttached && *disk.InstanceId != instanceId {
				return nil, status.Error(codes.FailedPrecondition, "disk is attach to another instance already")
			}
		}
	}

	attachDiskRequest := cbs.NewAttachDisksRequest()
	attachDiskRequest.DiskIds = []*string{&diskId}
	attachDiskRequest.InstanceId = &instanceId

	_, err = ctrl.cbsClient.AttachDisks(attachDiskRequest)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	ticker := time.NewTicker(time.Second * 5)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*120)
	defer cancel()

	for {
		select {
		case <-ticker.C:
			listCbsRequest := cbs.NewDescribeDisksRequest()
			listCbsRequest.DiskIds = []*string{&diskId}

			listCbsResponse, err := ctrl.cbsClient.DescribeDisks(listCbsRequest)
			if err != nil {
				continue
			}
			if len(listCbsResponse.Response.DiskSet) >= 1 {
				for _, d := range listCbsResponse.Response.DiskSet {
					if *d.DiskId == diskId && d.DiskState != nil {
						if *d.DiskState == StatusAttached {
							return &csi.ControllerPublishVolumeResponse{}, nil
						}
					}
				}
			}
		case <-ctx.Done():
			return nil, status.Error(codes.DeadlineExceeded, "cbs disk is not attached before deadline exceeded")
		}
	}
}

func (ctrl *cbsController) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume id is empty")
	}
	if req.NodeId == "" {
		return nil, status.Error(codes.InvalidArgument, "node id is empty")
	}

	diskId := req.VolumeId

	listCbsRequest := cbs.NewDescribeDisksRequest()
	listCbsRequest.DiskIds = []*string{&diskId}
	updateCbsClent(ctrl.cbsClient)
	listCbsResponse, err := ctrl.cbsClient.DescribeDisks(listCbsRequest)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if len(listCbsResponse.Response.DiskSet) <= 0 {
		// return nil, status.Error(codes.NotFound, "disk not found")
		glog.Warningf("ControllerUnpublishVolume: detach disk %s from node %s, but cbs disk does not exist; assuming the disk is detached", diskId, req.NodeId)
		return &csi.ControllerUnpublishVolumeResponse{}, nil
	}

	for _, disk := range listCbsResponse.Response.DiskSet {
		if *disk.DiskId == diskId {
			if *disk.DiskState == StatusUnattached {
				return &csi.ControllerUnpublishVolumeResponse{}, nil
			}
		}
	}

	detachDiskRequest := cbs.NewDetachDisksRequest()
	detachDiskRequest.DiskIds = []*string{&diskId}

	_, err = ctrl.cbsClient.DetachDisks(detachDiskRequest)
	if err != nil {
		return nil, status.Error(codes.Aborted, err.Error())
	}

	ticker := time.NewTicker(time.Second * 5)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*120)
	defer cancel()

	for {
		select {
		case <-ticker.C:
			listCbsRequest := cbs.NewDescribeDisksRequest()
			listCbsRequest.DiskIds = []*string{&diskId}

			listCbsResponse, err := ctrl.cbsClient.DescribeDisks(listCbsRequest)
			if err != nil {
				continue
			}
			if len(listCbsResponse.Response.DiskSet) >= 1 {
				for _, d := range listCbsResponse.Response.DiskSet {
					if *d.DiskId == diskId && d.DiskState != nil {
						if *d.DiskState == StatusUnattached {
							return &csi.ControllerUnpublishVolumeResponse{}, nil
						}
					}
				}
			}
		case <-ctx.Done():
			return nil, status.Error(codes.DeadlineExceeded, "cbs disk is not unattached before deadline exceeded")
		}
	}
}

func (ctrl *cbsController) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	glog.Infof("ControllerGetCapabilities: called with args %+v", *req)
	var caps []*csi.ControllerServiceCapability
	for _, cap := range controllerCaps {
		c := &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: cap,
				},
			},
		}
		caps = append(caps, c)
	}
	return &csi.ControllerGetCapabilitiesResponse{Capabilities: caps}, nil
}

func (ctrl *cbsController) ValidateVolumeCapabilities(context.Context, *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (ctrl *cbsController) ListVolumes(context.Context, *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (ctrl *cbsController) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	glog.Infof("ControllerExpandVolume: ControllerExpandVolumeRequest is %v", *req)

	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	capacityRange := req.GetCapacityRange()
	if capacityRange == nil {
		return nil, status.Error(codes.InvalidArgument, "Capacity range not provided")
	}

	newCbsSizeGB := util.RoundUpGiB(capacityRange.GetRequiredBytes())

	diskId := req.VolumeId

	// check size
	listCbsRequest := cbs.NewDescribeDisksRequest()
	listCbsRequest.DiskIds = []*string{&diskId}
	updateCbsClent(ctrl.cbsClient)
	listCbsResponse, err := ctrl.cbsClient.DescribeDisks(listCbsRequest)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if len(listCbsResponse.Response.DiskSet) <= 0 {
		return nil, status.Errorf(codes.NotFound, "Disk %s not found", diskId)
	}

	for _, disk := range listCbsResponse.Response.DiskSet {
		if *disk.DiskId == diskId {
			if uint64(newCbsSizeGB) <= *disk.DiskSize {
				glog.Infof("Request size %v is less than *disk.DiskSize %v", newCbsSizeGB, *disk.DiskSize)
				return &csi.ControllerExpandVolumeResponse{
					CapacityBytes:         util.GiBToBytes(int64(*disk.DiskSize)),
					NodeExpansionRequired: true,
				}, nil
			}
		}
	}
	//expand cbs
	resizeRequest := cbs.NewResizeDiskRequest()
	resizeRequest.DiskId = common.StringPtr(diskId)
	resizeRequest.DiskSize = common.Uint64Ptr(uint64(newCbsSizeGB))

	_, err = ctrl.cbsClient.ResizeDisk(resizeRequest)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	ticker := time.NewTicker(time.Second * 3)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*120)
	defer cancel()

	for {
		select {
		case <-ticker.C:
			listCbsResponse, err := ctrl.cbsClient.DescribeDisks(listCbsRequest)
			if err != nil {
				glog.Infof("DescribeDisks failed %v", err)
				continue
			}
			for _, d := range listCbsResponse.Response.DiskSet {
				if *d.DiskId == diskId && d.DiskState != nil {
					if *d.DiskState != StatusExpanding {
						return &csi.ControllerExpandVolumeResponse{
							CapacityBytes:         util.GiBToBytes(newCbsSizeGB),
							NodeExpansionRequired: true,
						}, nil
					}
				}
			}
		case <-ctx.Done():
			return nil, status.Errorf(codes.DeadlineExceeded, "Cbs disk %s is not expanded before deadline exceeded", diskId)
		}
	}
}

func (ctrl *cbsController) GetCapacity(context.Context, *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (ctrl *cbsController) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	if err := ctrl.validateSnapshotReq(req); err != nil {
		return nil, err
	}
	sourceVolumeId := req.GetSourceVolumeId()
	snapshotName := req.GetName()
	updateCbsClent(ctrl.cbsClient)

	if cbsSnap, err := getCbsSnapshotByName(snapshotName); err == nil {
		listSnapshotRequest := cbs.NewDescribeSnapshotsRequest()
		listSnapshotRequest.SnapshotIds = []*string{&cbsSnap.SnapId}
		listSnapshotResponse, err := ctrl.cbsClient.DescribeSnapshots(listSnapshotRequest)

		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		if len(listSnapshotResponse.Response.SnapshotSet) >= 0 {
			for _, s := range listSnapshotResponse.Response.SnapshotSet {
				if *s.DiskId == cbsSnap.SourceVolumeId {
					if err = ctrl.metadataStore.Create(cbsSnap.SnapId, cbsSnap); err != nil {
						glog.Errorf("failed to restore metadata for snapshot %s: %v", cbsSnap.SnapId, err)
						return nil, status.Error(codes.Internal, err.Error())
					}
					if *s.SnapshotState == SnapshotNormal && *s.Percent == 100 {
						return &csi.CreateSnapshotResponse{
							Snapshot: &csi.Snapshot{
								SizeBytes:      cbsSnap.SizeBytes,
								SnapshotId:     cbsSnap.SnapId,
								SourceVolumeId: cbsSnap.SourceVolumeId,
								CreationTime: &timestamp.Timestamp{
									Seconds: cbsSnap.CreatedAt,
								},
								ReadyToUse: true,
							},
						}, nil
					}
				}
			}
			return &csi.CreateSnapshotResponse{
				Snapshot: &csi.Snapshot{
					SizeBytes:      cbsSnap.SizeBytes,
					SnapshotId:     cbsSnap.SnapId,
					SourceVolumeId: cbsSnap.SourceVolumeId,
					CreationTime: &timestamp.Timestamp{
						Seconds: cbsSnap.CreatedAt,
					},
					ReadyToUse: false,
				},
			}, nil
		}
		return nil, status.Error(codes.NotFound, err.Error())
	}

	createSnapRequest := cbs.NewCreateSnapshotRequest()
	createSnapRequest.DiskId = &sourceVolumeId
	createSnapRequest.SnapshotName = &snapshotName
	createSnpResponse, err := ctrl.cbsClient.CreateSnapshot(createSnapRequest)

	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	snapshotId := *createSnpResponse.Response.SnapshotId
	listSnapshotRequest := cbs.NewDescribeSnapshotsRequest()
	listSnapshotRequest.SnapshotIds = []*string{&snapshotId}
	listSnapshotResponse, err := ctrl.cbsClient.DescribeSnapshots(listSnapshotRequest)

	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if len(listSnapshotResponse.Response.SnapshotSet) <= 0 {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	cbsSnap := &cbsSnapshot{
		SnapId:         snapshotId,
		SnapName:       snapshotName,
		SourceVolumeId: sourceVolumeId,
		SizeBytes:      int64(*(listSnapshotResponse.Response.SnapshotSet[0].DiskSize) * GB),
		CreatedAt:      ptypes.TimestampNow().GetSeconds(),
	}

	cbsSnapshotsMapsCache.add(snapshotId, cbsSnap)

	if err = ctrl.metadataStore.Create(snapshotId, cbsSnap); err != nil {
		glog.Errorf("failed to store metadata for snapshot %s: %v", snapshotId, cbsSnap)
		return nil, status.Error(codes.Internal, err.Error())
	}

	glog.Infof("req.getSnapshotName is %s", req.GetName())

	return &csi.CreateSnapshotResponse{
		Snapshot: &csi.Snapshot{
			SizeBytes:      cbsSnap.SizeBytes,
			SnapshotId:     cbsSnap.SnapId,
			SourceVolumeId: cbsSnap.SourceVolumeId,
			CreationTime: &timestamp.Timestamp{
				Seconds: cbsSnap.CreatedAt,
			},
			ReadyToUse: false,
		},
	}, nil
}

func (ctrl *cbsController) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {

	snapshotId := req.GetSnapshotId()
	if len(snapshotId) == 0 {
		return nil, status.Error(codes.InvalidArgument, "snapshot id is empty")
	}

	cbsSnap := &cbsSnapshot{}
	if err := ctrl.metadataStore.Get(snapshotId, cbsSnap); err != nil {
		if err, ok := err.(*util.CacheEntryNotFound); ok {
			glog.Infof("metadata for snapshot %s not found, assuming the snapshot to be already deleted (%v)", snapshotId, err)
			return &csi.DeleteSnapshotResponse{}, nil
		}

		return nil, status.Error(codes.Internal, err.Error())
	}

	terminateSnapRequest := cbs.NewDeleteSnapshotsRequest()
	terminateSnapRequest.SnapshotIds = []*string{&snapshotId}
	updateCbsClent(ctrl.cbsClient)
	_, err := ctrl.cbsClient.DeleteSnapshots(terminateSnapRequest)

	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	cbsSnapshotsMapsCache.delete(snapshotId)

	if err := ctrl.metadataStore.Delete(snapshotId); err != nil {
		glog.Errorf("delete metadata for snapshot %s failed for :%v", snapshotId, err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.DeleteSnapshotResponse{}, nil
}

func (ctrl *cbsController) ListSnapshots(context.Context, *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func pickAvailabilityZone(requirement *csi.TopologyRequirement) string {
	if requirement == nil {
		return ""
	}
	for _, topology := range requirement.GetPreferred() {
		zone, exists := topology.GetSegments()[TopologyZoneKey]
		if exists {
			return zone
		}
	}
	for _, topology := range requirement.GetRequisite() {
		zone, exists := topology.GetSegments()[TopologyZoneKey]
		if exists {
			return zone
		}
	}
	return ""
}

func (ctrl *cbsController) validateSnapshotReq(req *csi.CreateSnapshotRequest) error {
	// Check sanity of request Snapshot Name, Source Volume Id
	if len(req.Name) == 0 {
		return status.Error(codes.InvalidArgument, "Snapshot Name cannot be empty")
	}
	if len(req.SourceVolumeId) == 0 {
		return status.Error(codes.InvalidArgument, "Source Volume ID cannot be empty")
	}
	return nil
}

// LoadExDataFromMetadataStore loads the cbs snapshot
// info from metadata store
func (ctrl *cbsController) LoadExDataFromMetadataStore() error {
	snap := &cbsSnapshot{}
	// nolint
	ctrl.metadataStore.ForAll("snap-(.*)", snap, func(identifier string) error {
		cbsSnapshotsMapsCache.add(identifier, snap)
		return nil
	})

	glog.Infof("Loaded %d snapshots from metadata store", len(cbsSnapshotsMapsCache.cbsSnapshotMaps))
	return nil
}

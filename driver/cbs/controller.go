package cbs

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
	cbs "github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/cbs/v20170312"
	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/metrics"
	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/util"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	cvm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
	tag "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/tag/v20180813"
)

const (
	GB uint64 = 1 << (10 * 3)

	DiskTypeCloudBasic   = "CLOUD_BASIC"
	DiskTypeCloudPremium = "CLOUD_PREMIUM"
	DiskTypeCloudHSSD    = "CLOUD_HSSD"
	DiskTypeCloudTSSD    = "CLOUD_TSSD"

	DiskChargeTypePrePaid        = "PREPAID"
	DiskChargeTypePostPaidByHour = "POSTPAID_BY_HOUR"
	DiskChargeTypeCdcPaid        = "CDCPAID"

	DiskChargePrepaidRenewFlagNotifyAndAutoRenew          = "NOTIFY_AND_AUTO_RENEW"
	DiskChargePrepaidRenewFlagNotifyAndManualRenew        = "NOTIFY_AND_MANUAL_RENEW"
	DiskChargePrepaidRenewFlagDisableNotifyAndManualRenew = "DISABLE_NOTIFY_AND_MANUAL_RENEW"

	TKESERVICETYPE          = "ccs"
	TKERESOURCEPREFIX       = "cluster"
	TagForDeletionCreateBy  = "tke-cbs-provisioner-createBy-flag"
	TagForDeletionClusterId = "tke-clusterId"

	StatusUnattached = "UNATTACHED"
	StatusAttached   = "ATTACHED"
	StatusExpanding  = "EXPANDING"

	SnapshotNormal   = "NORMAL"
	SnapShotNotFound = "InvalidSnapshotId.NotFound"
	CVMNodeIDPrefix  = "ins-"
	CXMNodeIDPrefix  = "eks-"
	NodeIDLength     = 12

	dmDefaultDevices = 2

	cbsUrl     = "cbs.internal.tencentcloudapi.com"
	cvmUrl     = "cvm.internal.tencentcloudapi.com"
	tagUrl     = "tag.internal.tencentcloudapi.com"
	cbsTestUrl = "cbs.test.tencentcloudapi.com"
	cvmTestUrl = "cvm.test.tencentcloudapi.com"
	tagTestUrl = "tag.test.tencentcloudapi.com"
)

var (
	EncryptAttr     = "encrypt"
	EncryptEnable   = "ENCRYPT"
	CXMInstanceType = "eks"

	DiskChargePrepaidPeriodValidValues = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 24, 36}

	cbsSnapshotsMapsCache = &cbsSnapshotsCache{
		mux:             &sync.Mutex{},
		cbsSnapshotMaps: make(map[string]*cbsSnapshot),
	}

	// controllerCaps represents the capability of controller service
	controllerCaps = []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
	}
)

type cbsController struct {
	cbsClient     *cbs.Client
	cvmClient     *cvm.Client
	tagClient     *tag.Client
	metadataStore util.CachePersister

	zone      string
	region    string
	clusterId string
}

func newCbsController(drv *Driver) *cbsController {
	secretID, secretKey, token, _ := util.GetSercet()
	cred := &common.Credential{
		SecretId:  secretID,
		SecretKey: secretKey,
		Token:     token,
	}

	cbsCpf := profile.NewClientProfile()
	cbsCpf.HttpProfile.Endpoint = cbsUrl

	cvmCpf := profile.NewClientProfile()
	cvmCpf.HttpProfile.Endpoint = cvmUrl

	tagCpf := profile.NewClientProfile()
	tagCpf.Language = "en-US"
	tagCpf.HttpProfile.Endpoint = tagUrl

	if drv.environmentType == "test" {
		cbsCpf.HttpProfile.Endpoint = cbsTestUrl
		cvmCpf.HttpProfile.Endpoint = cvmTestUrl
		tagCpf.HttpProfile.Endpoint = tagTestUrl
	}

	if drv.cbsUrl != "" {
		cbsCpf.HttpProfile.Endpoint = drv.cbsUrl
	}

	cbsClient, _ := cbs.NewClient(cred, drv.region, cbsCpf)
	cvmClient, _ := cvm.NewClient(cred, drv.region, cvmCpf)
	tagClient, _ := tag.NewClient(cred, drv.region, tagCpf)

	return &cbsController{
		cbsClient:     cbsClient,
		cvmClient:     cvmClient,
		tagClient:     tagClient,
		zone:          drv.zone,
		region:        drv.region,
		clusterId:     drv.clusterId,
		metadataStore: drv.metadataStore,
	}
}

func (ctrl *cbsController) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "volume name is empty")
	}
	if len(req.VolumeCapabilities) <= 0 {
		return nil, status.Error(codes.InvalidArgument, "volume has no capabilities")
	}
	for _, c := range req.VolumeCapabilities {
		if c.AccessMode.Mode != csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER {
			return nil, status.Error(codes.InvalidArgument, "cbs access mode only support singer node writer")
		}
	}
	updateClient(ctrl.cbsClient, ctrl.cvmClient, ctrl.tagClient)

	volumeIdempotencyName := req.Name
	volumeCapacity := req.CapacityRange.RequiredBytes

	volumeTags := make(map[string]string)
	if ctrl.clusterId != "" {
		volumeTags[TagForDeletionCreateBy] = "yes"
		volumeTags[TagForDeletionClusterId] = ctrl.clusterId
	}

	var aspId, volumeZone, cdcId, devicemapper string
	var projectId, throughputPerformance int
	var err error
	inputVolumeType := DiskTypeCloudPremium
	volumeChargeType := DiskChargeTypePostPaidByHour
	volumeChargePrepaidRenewFlag := DiskChargePrepaidRenewFlagNotifyAndManualRenew
	volumeChargePrepaidPeriod := 1
	devices := 1
	for k, v := range req.Parameters {
		switch strings.ToLower(k) {
		case "aspid":
			aspId = v
		case "type", "disktype":
			inputVolumeType = v
		case "zone", "diskzone":
			volumeZone = v
		case "paymode", "diskchargetype":
			volumeChargeType = v
		case "renewflag", "diskchargeprepaidrenewflag":
			volumeChargePrepaidRenewFlag = v
		case "diskchargetypeprepaidperiod":
			volumeChargePrepaidPeriod, err = strconv.Atoi(v)
			if err != nil {
				glog.Warningf("volumeChargePrepaidPeriod atoi error: %v", err)
			}
		case "project":
			projectId, err = strconv.Atoi(v)
			if err != nil {
				glog.Warningf("projectId atoi error: %v", err)
			}
		case "disktags":
			diskTags := strings.Split(v, ",")
			for _, diskTag := range diskTags {
				kv := strings.Split(diskTag, ":")
				if kv == nil || len(kv) != 2 {
					continue
				}
				volumeTags[kv[0]] = kv[1]
			}
		case "throughputperformance":
			throughputPerformance, err = strconv.Atoi(v)
			if err != nil {
				glog.Warningf("throughputPerformance atoi error: %v", err)
			}
		case "cdcid":
			cdcId = v
			volumeChargeType = DiskChargeTypeCdcPaid
		case "devicemapper":
			devicemapper = v
		case "devices":
			devices, err = strconv.Atoi(v)
			if err != nil {
				glog.Warningf("volumeChargePrepaidPeriod atoi error: %v", err)
			}
			if devices <= 0 {
				return nil, status.Errorf(codes.InvalidArgument, "devices %v is invalid", devices)
			}
		default:
		}
	}

	if devicemapper != "" {
		if devicemapper != "LVM" {
			return nil, status.Errorf(codes.InvalidArgument, "devicemapper %s is unsupported", devicemapper)
		}
		if devices < dmDefaultDevices {
			return nil, status.Errorf(codes.InvalidArgument, "devices can't less then 2 when devicemapper is %s", devicemapper)
		}
		glog.Infof("devicemapper %s enabled，devices: %v", devicemapper, devices)
	} else if devices != 1 {
		return nil, status.Error(codes.InvalidArgument, "devices must set to 1 when devicemapper is empty")
	}

	if volumeZone == "" {
		volumeZone = pickAvailabilityZone(req.GetAccessibilityRequirements())
	}
	if volumeZone == "" {
		volumeZone = ctrl.zone
	}

	glog.Infof("tencent zone: %v", volumeZone)
	if !strings.HasPrefix(volumeZone, "ap-") {
		request := cvm.NewDescribeZonesRequest()
		response, err := ctrl.cvmClient.DescribeZones(request)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "cvm.DescribeZones failed, err: %v", err.Error())
		}
		for _, z := range response.Response.ZoneSet {
			if *z.ZoneId == volumeZone {
				glog.Infof("volume cbs zone convert %v to %v", volumeZone, *z.Zone)
				volumeZone = *z.Zone
				break
			}
		}
	}

	if volumeChargeType == DiskChargeTypePrePaid {
		found := false
		for _, p := range DiskChargePrepaidPeriodValidValues {
			if p == volumeChargePrepaidPeriod {
				found = true
			}
		}
		if !found {
			metrics.CbsPvcsRequestTotal.WithLabelValues(DriverName, string(util.Provision), string(util.InvalidArgs)).Inc()
			return nil, status.Error(codes.InvalidArgument, "can not found valid prepaid period")
		}
		if volumeChargePrepaidRenewFlag != DiskChargePrepaidRenewFlagDisableNotifyAndManualRenew && volumeChargePrepaidRenewFlag != DiskChargePrepaidRenewFlagNotifyAndAutoRenew &&
			volumeChargePrepaidRenewFlag != DiskChargePrepaidRenewFlagNotifyAndManualRenew {
			metrics.CbsPvcsRequestTotal.WithLabelValues(DriverName, string(util.Provision), string(util.InvalidArgs)).Inc()
			return nil, status.Error(codes.InvalidArgument, "invalid renew flag")
		}
	}

	size := uint64(volumeCapacity / int64(GB))
	volumeType, err := ctrl.validateDiskTypeAndSize(inputVolumeType, volumeZone, volumeChargeType, size)
	if err != nil {
		metrics.CbsPvcsRequestTotal.WithLabelValues(DriverName, string(util.Provision), string(util.InvalidArgs)).Inc()
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if volumeType == "" {
		metrics.CbsPvcsRequestTotal.WithLabelValues(DriverName, string(util.Provision), util.ErrDiskTypeNotAvaliable.Code).Inc()
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("no available storage in zone: %s", volumeZone))
	}

	volumeEncrypt, ok := req.Parameters[EncryptAttr]
	if !ok {
		volumeEncrypt = ""
	}
	if volumeEncrypt != "" && volumeEncrypt != EncryptEnable {
		metrics.CbsPvcsRequestTotal.WithLabelValues(DriverName, string(util.Provision), string(util.InvalidArgs)).Inc()
		return nil, status.Error(codes.InvalidArgument, "volume encrypt not valid")
	}

	diskName := volumeIdempotencyName
	if ctrl.clusterId != "" {
		diskName = fmt.Sprintf("%s/%s", ctrl.clusterId, volumeIdempotencyName)
	}

	createCbsReq := cbs.NewCreateDisksRequest()
	createCbsReq.DiskName = common.StringPtr(diskName)
	createCbsReq.ClientToken = common.StringPtr(diskName)
	createCbsReq.DiskType = &volumeType
	createCbsReq.DiskChargeType = &volumeChargeType
	createCbsReq.DiskSize = &size

	if (volumeType == DiskTypeCloudHSSD || volumeType == DiskTypeCloudTSSD) && throughputPerformance != 0 {
		tpUnit64 := uint64(throughputPerformance)
		createCbsReq.ThroughputPerformance = &tpUnit64
	}

	if volumeChargeType == DiskChargeTypePrePaid {
		period := uint64(volumeChargePrepaidPeriod)
		createCbsReq.DiskChargePrepaid = &cbs.DiskChargePrepaid{
			Period:    &period,
			RenewFlag: &volumeChargePrepaidRenewFlag,
		}
	}

	if volumeEncrypt == EncryptEnable {
		createCbsReq.Encrypt = &EncryptEnable
	}

	createCbsReq.Placement = &cbs.Placement{
		Zone:      &volumeZone,
		ProjectId: common.Uint64Ptr(uint64(projectId)),
	}

	if cdcId != "" {
		createCbsReq.Placement.CdcId = &cdcId
	}

	snapshotId := ""
	if req.VolumeContentSource != nil {
		sTimeForDescribeSnapshots := time.Now()
		snapshot := req.VolumeContentSource.GetSnapshot()
		if snapshot == nil {
			metrics.CbsPvcsRequestTotal.WithLabelValues(DriverName, string(util.Provision), string(util.InvalidArgs)).Inc()
			return nil, status.Error(codes.InvalidArgument, "Volume Snapshot cannot be empty")
		}
		snapshotId = snapshot.GetSnapshotId()
		if len(snapshotId) == 0 {
			metrics.CbsPvcsRequestTotal.WithLabelValues(DriverName, string(util.Provision), string(util.InvalidArgs)).Inc()
			return nil, status.Error(codes.InvalidArgument, "Volume Snapshot ID cannot be empty")
		}
		listSnapshotRequest := cbs.NewDescribeSnapshotsRequest()
		listSnapshotRequest.SnapshotIds = []*string{&snapshotId}
		listSnapshotResponse, err := ctrl.cbsClient.DescribeSnapshots(listSnapshotRequest)
		if err != nil {
			metrics.YunApiRequestTotal.WithLabelValues(DriverName, string(util.CBS), string(util.DescribeSnapshots), "", util.GetTencentSdkErrCode(err)).Inc()
			metrics.CbsPvcsRequestTotal.WithLabelValues(DriverName, string(util.Provision), util.GetTencentSdkErrCode(err)).Inc()
			return nil, status.Error(codes.Internal, err.Error())
		}
		metrics.YunApiRequestTotal.WithLabelValues(DriverName, string(util.CBS), string(util.DescribeSnapshots), "", string(util.Success)).Inc()
		metrics.YunApiRequestCostSeconds.WithLabelValues(DriverName, string(util.CBS), string(util.DescribeSnapshots)).Observe(time.Since(sTimeForDescribeSnapshots).Seconds())
		if len(listSnapshotResponse.Response.SnapshotSet) <= 0 {
			metrics.CbsPvcsRequestTotal.WithLabelValues(DriverName, string(util.Provision), util.ErrSnapshotNotFound.Code).Inc()
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

	tagRequest := tag.NewDescribeResourceTagsByResourceIdsRequest()
	tagRequest.ServiceType = common.StringPtr(TKESERVICETYPE)
	tagRequest.ResourcePrefix = common.StringPtr(TKERESOURCEPREFIX)
	tagRequest.ResourceIds = common.StringPtrs([]string{ctrl.clusterId})
	tagRequest.ResourceRegion = common.StringPtr(ctrl.region)
	tagRequest.Limit = common.Uint64Ptr(100)
	tagResp, err := ctrl.tagClient.DescribeResourceTagsByResourceIds(tagRequest)
	if err != nil {
		glog.Warningf("DescribeResourceTagsByResourceIds error %v", err)
	} else if tagResp.Response != nil && len(tagResp.Response.Tags) > 0 {
		glog.Infof("cluster %v's tags count is %v", ctrl.clusterId, *tagResp.Response.TotalCount)
		for _, clusterTag := range tagResp.Response.Tags {
			volumeTags[*clusterTag.TagKey] = *clusterTag.TagValue
		}
	}
	for k, v := range volumeTags {
		key := k
		value := v
		createCbsReq.Tags = append(createCbsReq.Tags, &cbs.Tag{
			Key:   &key,
			Value: &value,
		})
	}

	createCbsReq.DiskCount = common.Uint64Ptr(uint64(devices))

	glog.Infof("createCbsReq: %s", createCbsReq.ToJsonString())
	sTimeForCreateDisks := time.Now()
	createCbsResponse, err := ctrl.cbsClient.CreateDisks(createCbsReq)
	if err != nil {
		metrics.YunApiRequestTotal.WithLabelValues(DriverName, string(util.CBS), string(util.CreateDisks), "", util.GetTencentSdkErrCode(err)).Inc()
		metrics.CbsPvcsRequestTotal.WithLabelValues(DriverName, string(util.Provision), util.GetTencentSdkErrCode(err)).Inc()
		return nil, status.Error(codes.Internal, err.Error())
	}
	metrics.YunApiRequestTotal.WithLabelValues(DriverName, string(util.CBS), string(util.CreateDisks), "", string(util.Success)).Inc()
	metrics.YunApiRequestCostSeconds.WithLabelValues(DriverName, string(util.CBS), string(util.CreateDisks)).Observe(time.Since(sTimeForCreateDisks).Seconds())

	if len(createCbsResponse.Response.DiskIdSet) < devices {
		metrics.CbsPvcsRequestTotal.WithLabelValues(DriverName, string(util.Provision), string(util.InternalErr)).Inc()
		return nil, status.Errorf(codes.Internal, "create disk failed, not enough disk id found in create disk response, request id %s", *createCbsResponse.Response.RequestId)
	}

	diskIds := createCbsResponse.Response.DiskIdSet
	var diskIdsStr string
	for idx, disk := range diskIds {
		if idx == len(diskIds)-1 {
			diskIdsStr += *disk
		} else {
			diskIdsStr += *disk + ","
		}
	}

	createVolumeResponses := make(chan error, len(diskIds))
	wg := &sync.WaitGroup{}
	for _, diskId := range diskIds {
		wg.Add(1)
		go ctrl.createVolume(*diskId, aspId, createVolumeResponses, wg)
	}
	wg.Wait()

	close(createVolumeResponses)
	respErrs := make([]error, 0)
	for r := range createVolumeResponses {
		if r != nil {
			respErrs = append(respErrs, r)
		}
	}
	if len(respErrs) > 0 {
		return nil, status.Errorf(codes.Aborted, "createVolume %s failed, err: %v", diskIdsStr, respErrs)
	}

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			CapacityBytes: volumeCapacity,
			ContentSource: src,
			VolumeId:      diskIdsStr,
			VolumeContext: req.GetParameters(),
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

func (ctrl *cbsController) createVolume(diskId, aspId string, createVolumeResponses chan error, wg *sync.WaitGroup) {
	defer wg.Done()

	disk := new(cbs.Disk)
	ticker := time.NewTicker(time.Second * 5)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*120)
	defer cancel()
	for {
		select {
		case <-ticker.C:
			describeDiskResponse, err := ctrl.describeDisks([]*string{&diskId}, "", util.Provision)
			if err != nil {
				glog.Warningf("createVolume %s DescribeDisks failed, err: %v", diskId, err)
				continue
			}
			if len(describeDiskResponse.Response.DiskSet) >= 1 {
				for _, d := range describeDiskResponse.Response.DiskSet {
					if *d.DiskId == diskId && d.DiskState != nil {
						if *d.DiskState == StatusAttached || *d.DiskState == StatusUnattached {
							disk = d
							if aspId != "" {
								glog.Infof("try to bind disk %s to aspId: %s", diskId, aspId)
								bindReq := cbs.NewBindAutoSnapshotPolicyRequest()
								bindReq.AutoSnapshotPolicyId = &aspId
								bindReq.DiskIds = []*string{disk.DiskId}
								basp, err := ctrl.cbsClient.BindAutoSnapshotPolicy(bindReq)
								if err != nil {
									glog.Warningf("failed to bind snapshot policy, err: %v", err)
								} else {
									if basp.Response != nil && basp.Response.RequestId != nil {
										glog.Infof("success bind disk %s to aspId: %s, the requestID is %s", diskId, aspId, *basp.Response.RequestId)
									}

								}
							}
							// record metric for create volume successfully
							metrics.CbsPvcsRequestTotal.WithLabelValues(DriverName, string(util.Provision), string(util.Success)).Inc()
							createVolumeResponses <- nil
							return
						}
					}

				}
			}
		case <-ctx.Done():
			metrics.CbsPvcsRequestTotal.WithLabelValues(DriverName, string(util.Provision), util.ErrDiskCreatedTimeout.Code).Inc()
			createVolumeResponses <- status.Error(codes.DeadlineExceeded, fmt.Sprintf("cbs disk %s is not ready before deadline exceeded", diskId))
			return
		}
	}
}

func (ctrl *cbsController) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume id is empty")
	}
	updateClient(ctrl.cbsClient, ctrl.cvmClient, ctrl.tagClient)

	diskIdList := make([]*string, 0)
	for _, disk := range strings.Split(req.VolumeId, ",") {
		if strings.HasPrefix(disk, "disk-") {
			diskId := disk
			diskIdList = append(diskIdList, &diskId)
		}
	}

	describeDiskResponse, err := ctrl.describeDisks(diskIdList, "", util.Delete)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if len(describeDiskResponse.Response.DiskSet) <= 0 {
		return &csi.DeleteVolumeResponse{}, nil
	}

	sTimeForTerminateDisks := time.Now()
	terminateCbsRequest := cbs.NewTerminateDisksRequest()
	terminateCbsRequest.DiskIds = diskIdList
	_, err = ctrl.cbsClient.TerminateDisks(terminateCbsRequest)
	if err != nil {
		metrics.YunApiRequestTotal.WithLabelValues(DriverName, string(util.CBS), string(util.TerminateDisks), "", util.GetTencentSdkErrCode(err)).Inc()
		metrics.CbsPvcsRequestTotal.WithLabelValues(DriverName, string(util.Delete), util.GetTencentSdkErrCode(err)).Inc()
		return nil, status.Error(codes.Internal, err.Error())
	}
	metrics.YunApiRequestTotal.WithLabelValues(DriverName, string(util.CBS), string(util.TerminateDisks), "", string(util.Success)).Inc()
	metrics.YunApiRequestCostSeconds.WithLabelValues(DriverName, string(util.CBS), string(util.TerminateDisks)).Observe(time.Since(sTimeForTerminateDisks).Seconds())
	metrics.CbsPvcsRequestTotal.WithLabelValues(DriverName, string(util.Delete), string(util.Success)).Inc()

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
	updateClient(ctrl.cbsClient, ctrl.cvmClient, ctrl.tagClient)

	diskIds := req.VolumeId
	instanceId := req.NodeId

	if !((strings.HasPrefix(instanceId, CVMNodeIDPrefix) || strings.HasPrefix(instanceId, CXMNodeIDPrefix)) && len(instanceId) == NodeIDLength) {
		return nil, status.Errorf(codes.Internal, "attach disk %s to node %s, but the node's instanceType is unsupported", diskIds, instanceId)
	}

	diskIdList := make([]string, 0)
	for _, disk := range strings.Split(diskIds, ",") {
		if strings.HasPrefix(disk, "disk-") {
			diskIdList = append(diskIdList, disk)
		}
	}
	if len(diskIds) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "volume id %s is invalid", diskIds)
	}

	attachVolumeResponses := make(chan error, len(diskIdList))
	wg := &sync.WaitGroup{}
	for _, diskId := range diskIdList {
		wg.Add(1)
		go ctrl.attachVolume(diskId, req.NodeId, attachVolumeResponses, wg)
	}
	wg.Wait()

	close(attachVolumeResponses)
	respErrs := make([]error, 0)
	for r := range attachVolumeResponses {
		if r != nil {
			respErrs = append(respErrs, r)
		}
	}
	if len(respErrs) > 0 {
		return nil, status.Errorf(codes.Aborted, "attachVolume %s failed, err: %v", diskIds, respErrs)
	}

	return &csi.ControllerPublishVolumeResponse{}, nil
}

func (ctrl *cbsController) attachVolume(diskId, instanceId string, attachVolumeResponses chan error, wg *sync.WaitGroup) {
	defer wg.Done()

	describeDiskResponse, err := ctrl.describeDisks([]*string{&diskId}, instanceId, util.Attach)
	if err != nil {
		attachVolumeResponses <- status.Error(codes.Internal, err.Error())
		return
	}
	if len(describeDiskResponse.Response.DiskSet) <= 0 {
		metrics.OperationErrorsTotal.WithLabelValues(DriverName, string(util.Attach), diskId, instanceId, util.ErrDiskNotFound.Code).Inc()
		attachVolumeResponses <- status.Errorf(codes.NotFound, "disk %s not found", diskId)
		return
	}
	for _, disk := range describeDiskResponse.Response.DiskSet {
		if *disk.DiskId == diskId {
			if *disk.DiskState == StatusAttached && *disk.InstanceId == instanceId {
				attachVolumeResponses <- nil
				return
			}
			if *disk.DiskState == StatusAttached && *disk.InstanceId != instanceId {
				metrics.OperationErrorsTotal.WithLabelValues(DriverName, string(util.Attach), diskId, instanceId, util.ErrDiskAttachedAlready.Code).Inc()
				attachVolumeResponses <- status.Errorf(codes.FailedPrecondition, "disk %s is attach to another instance() already", diskId, *disk.InstanceId)
				return
			}
		}
	}

	attachDiskRequest := cbs.NewAttachDisksRequest()
	attachDiskRequest.DiskIds = []*string{&diskId}
	attachDiskRequest.InstanceId = &instanceId
	if strings.HasPrefix(instanceId, CXMNodeIDPrefix) {
		attachDiskRequest.InstanceType = &CXMInstanceType
	}
	sTimeForAttachDisks := time.Now()
	_, err = ctrl.cbsClient.AttachDisks(attachDiskRequest)
	if err != nil {
		metrics.YunApiRequestTotal.WithLabelValues(DriverName, string(util.CBS), string(util.AttachDisks), diskId, util.GetTencentSdkErrCode(err)).Inc()
		metrics.OperationErrorsTotal.WithLabelValues(DriverName, string(util.Attach), diskId, instanceId, util.GetTencentSdkErrCode(err)).Inc()
		attachVolumeResponses <- status.Error(codes.Internal, err.Error())
		return
	}
	metrics.YunApiRequestTotal.WithLabelValues(DriverName, string(util.CBS), string(util.AttachDisks), diskId, string(util.Success)).Inc()
	metrics.YunApiRequestCostSeconds.WithLabelValues(DriverName, string(util.CBS), string(util.AttachDisks)).Observe(time.Since(sTimeForAttachDisks).Seconds())

	ticker := time.NewTicker(time.Second * 5)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*120)
	defer cancel()
	for {
		select {
		case <-ticker.C:
			describeDiskResponse, err := ctrl.describeDisks([]*string{&diskId}, instanceId, "")
			if err != nil {
				glog.Warningf("attachVolume %s DescribeDisks failed, err: %v", diskId, err)
				continue
			}

			if len(describeDiskResponse.Response.DiskSet) >= 1 {
				for _, d := range describeDiskResponse.Response.DiskSet {
					if *d.DiskId == diskId && d.DiskState != nil {
						if *d.DiskState == StatusAttached {
							attachVolumeResponses <- nil
							return
						}
					}
				}
			}
		case <-ctx.Done():
			metrics.OperationErrorsTotal.WithLabelValues(DriverName, string(util.Attach), diskId, instanceId, util.ErrDiskAttachedTimeout.Code).Inc()
			attachVolumeResponses <- status.Errorf(codes.DeadlineExceeded, "cbs disk %s is not attached before deadline exceeded", diskId)
			return
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
	updateClient(ctrl.cbsClient, ctrl.cvmClient, ctrl.tagClient)

	diskIds := req.VolumeId
	instanceId := req.NodeId

	if !((strings.HasPrefix(instanceId, CVMNodeIDPrefix) || strings.HasPrefix(instanceId, CXMNodeIDPrefix)) && len(instanceId) == NodeIDLength) {
		glog.Infof("detach disk %s from node %s, but the node's instanceType is unsupported", diskIds, instanceId)
		return &csi.ControllerUnpublishVolumeResponse{}, nil
	}

	diskIdList := make([]string, 0)
	for _, disk := range strings.Split(diskIds, ",") {
		if strings.HasPrefix(disk, "disk-") {
			diskIdList = append(diskIdList, disk)
		}
	}
	if len(diskIds) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "volume id %s is invalid", diskIds)
	}

	detachVolumeResponses := make(chan error, len(diskIdList))
	wg := &sync.WaitGroup{}
	for _, diskId := range diskIdList {
		wg.Add(1)
		go ctrl.detachVolume(diskId, req.NodeId, detachVolumeResponses, wg)
	}
	wg.Wait()

	close(detachVolumeResponses)
	respErrs := make([]error, 0)
	for r := range detachVolumeResponses {
		if r != nil {
			respErrs = append(respErrs, r)
		}
	}
	if len(respErrs) > 0 {
		return nil, status.Errorf(codes.Aborted, "detachVolume %s failed, err: %v", diskIds, respErrs)
	}

	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (ctrl *cbsController) detachVolume(diskId, instanceId string, detachVolumeResponses chan error, wg *sync.WaitGroup) {
	defer wg.Done()

	describeDiskResponse, err := ctrl.describeDisks([]*string{&diskId}, instanceId, util.Detach)
	if err != nil {
		detachVolumeResponses <- status.Error(codes.Internal, err.Error())
		return
	}
	if len(describeDiskResponse.Response.DiskSet) <= 0 {
		glog.Warningf("detach disk %s from node %s, but cbs disk does not exist; assuming the disk is detached", diskId, instanceId)
		detachVolumeResponses <- nil
		return
	}
	for _, disk := range describeDiskResponse.Response.DiskSet {
		if *disk.DiskId == diskId {
			if *disk.DiskState == StatusUnattached {
				detachVolumeResponses <- nil
				return
			}
		}
	}

	detachDiskRequest := cbs.NewDetachDisksRequest()
	detachDiskRequest.DiskIds = []*string{&diskId}
	if strings.HasPrefix(instanceId, CXMNodeIDPrefix) {
		detachDiskRequest.InstanceType = &CXMInstanceType
	}
	sTimeForDetachDisks := time.Now()
	_, err = ctrl.cbsClient.DetachDisks(detachDiskRequest)
	if err != nil {
		metrics.YunApiRequestTotal.WithLabelValues(DriverName, string(util.CBS), string(util.DetachDisks), diskId, util.GetTencentSdkErrCode(err)).Inc()
		metrics.OperationErrorsTotal.WithLabelValues(DriverName, string(util.Detach), diskId, instanceId, util.GetTencentSdkErrCode(err)).Inc()
		detachVolumeResponses <- status.Error(codes.Internal, err.Error())
		return
	}
	metrics.YunApiRequestTotal.WithLabelValues(DriverName, string(util.CBS), string(util.DetachDisks), diskId, string(util.Success)).Inc()
	metrics.YunApiRequestCostSeconds.WithLabelValues(DriverName, string(util.CBS), string(util.DetachDisks)).Observe(time.Since(sTimeForDetachDisks).Seconds())

	ticker := time.NewTicker(time.Second * 5)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*120)
	defer cancel()
	for {
		select {
		case <-ticker.C:
			describeDiskResponse, err := ctrl.describeDisks([]*string{&diskId}, instanceId, "")
			if err != nil {
				glog.Warningf("detachVolume %s DescribeDisks failed, err: %v", diskId, err)
				continue
			}
			if len(describeDiskResponse.Response.DiskSet) >= 1 {
				for _, d := range describeDiskResponse.Response.DiskSet {
					if *d.DiskId == diskId && d.DiskState != nil {
						if *d.DiskState == StatusUnattached {
							detachVolumeResponses <- nil
							return
						}
					}
				}
			}
		case <-ctx.Done():
			metrics.OperationErrorsTotal.WithLabelValues(DriverName, string(util.Detach), diskId, instanceId, util.ErrDiskDetachedTimeout.Code).Inc()
			detachVolumeResponses <- status.Errorf(codes.DeadlineExceeded, "cbs disk %s is not unattached before deadline exceeded", diskId)
			return
		}
	}
}

func (ctrl *cbsController) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	glog.Infof("ControllerGetCapabilities: called with args %+v", *req)
	var caps []*csi.ControllerServiceCapability
	for _, controllerCap := range controllerCaps {
		c := &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: controllerCap,
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
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}
	if req.GetCapacityRange() == nil {
		return nil, status.Error(codes.InvalidArgument, "Capacity range not provided")
	}
	updateClient(ctrl.cbsClient, ctrl.cvmClient, ctrl.tagClient)

	newCbsSizeGB := util.RoundUpGiB(req.GetCapacityRange().GetRequiredBytes())

	diskIds := req.VolumeId
	diskIdList := make([]string, 0)
	for _, disk := range strings.Split(diskIds, ",") {
		if strings.HasPrefix(disk, "disk-") {
			diskIdList = append(diskIdList, disk)
		}
	}
	if len(diskIds) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "volume id %s is invalid", diskIds)
	}

	expandVolumeResponse := make(chan error, len(diskIdList))
	wg := &sync.WaitGroup{}
	for _, diskId := range diskIdList {
		wg.Add(1)
		go ctrl.expandVolume(diskId, newCbsSizeGB, expandVolumeResponse, wg)
	}
	wg.Wait()

	close(expandVolumeResponse)
	respErrs := make([]error, 0)
	for r := range expandVolumeResponse {
		if r != nil {
			respErrs = append(respErrs, r)
		}
	}
	if len(respErrs) > 0 {
		return nil, status.Errorf(codes.Aborted, "expandVolume %s failed, err: %v", diskIds, respErrs)
	}

	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         util.GiBToBytes(newCbsSizeGB),
		NodeExpansionRequired: true,
	}, nil
}

func (ctrl *cbsController) expandVolume(diskId string, newCbsSizeGB int64, expandVolumeResponse chan error, wg *sync.WaitGroup) {
	defer wg.Done()

	// check size
	describeDiskResponse, err := ctrl.describeDisks([]*string{&diskId}, "", "")
	if err != nil {
		expandVolumeResponse <- status.Error(codes.Internal, err.Error())
		return
	}

	if len(describeDiskResponse.Response.DiskSet) <= 0 {
		expandVolumeResponse <- status.Errorf(codes.NotFound, "disk %s not found", diskId)
		return
	}

	for _, disk := range describeDiskResponse.Response.DiskSet {
		if *disk.DiskId == diskId {
			if uint64(newCbsSizeGB) <= *disk.DiskSize {
				glog.Infof("request size %v is less than diskSize %v, disk %s", newCbsSizeGB, *disk.DiskSize, diskId)
				expandVolumeResponse <- nil
				return
			}
		}
	}

	//expand cbs
	resizeRequest := cbs.NewResizeDiskRequest()
	resizeRequest.DiskId = common.StringPtr(diskId)
	resizeRequest.DiskSize = common.Uint64Ptr(uint64(newCbsSizeGB))
	_, err = ctrl.cbsClient.ResizeDisk(resizeRequest)
	if err != nil {
		expandVolumeResponse <- status.Error(codes.Internal, err.Error())
		return
	}

	ticker := time.NewTicker(time.Second * 3)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*120)
	defer cancel()
	for {
		select {
		case <-ticker.C:
			describeDiskResponse, err := ctrl.describeDisks([]*string{&diskId}, "", "")
			if err != nil {
				glog.Warningf("expandVolume %s DescribeDisks failed, err: %v", diskId, err)
				continue
			}
			for _, d := range describeDiskResponse.Response.DiskSet {
				if *d.DiskId == diskId && d.DiskState != nil {
					if *d.DiskState != StatusExpanding {
						expandVolumeResponse <- nil
						return
					}
				}
			}
		case <-ctx.Done():
			expandVolumeResponse <- status.Errorf(codes.DeadlineExceeded, "cbs disk %s is not expanded before deadline exceeded", diskId)
			return
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
	if strings.Contains(sourceVolumeId, ",") {
		return nil, status.Error(codes.Internal, "create snapshot for devicemapper is unsupported")
	}

	snapshotName := req.GetName()
	updateClient(ctrl.cbsClient, ctrl.cvmClient, ctrl.tagClient)

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
	updateClient(ctrl.cbsClient, ctrl.cvmClient, ctrl.tagClient)
	_, err := ctrl.cbsClient.DeleteSnapshots(terminateSnapRequest)
	if err != nil {
		if sdkError, ok := err.(*errors.TencentCloudSDKError); ok {
			if sdkError.GetCode() == SnapShotNotFound {
				glog.Infof("snapshot %s not found, assuming the snapshot to be already deleted (%v)", snapshotId, err)
			} else {
				glog.Errorf("snapshot %s delete failed with TencentCloudSDKError error (%v)", snapshotId, err)
				return nil, status.Error(codes.Internal, err.Error())
			}
		} else {
			glog.Errorf("snapshot %s delete with error (%v)", snapshotId, err)
			return nil, status.Error(codes.Internal, err.Error())
		}
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

func (ctrl *cbsController) validateSnapshotReq(req *csi.CreateSnapshotRequest) error {
	// Check sanity of request Snapshot Name, Source Volume ID
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

func (ctrl *cbsController) validateDiskTypeAndSize(inputDiskType, zone, paymode string, diskSize uint64) (string, error) {
	sTime := time.Now()
	diskQuotaRequest := cbs.NewDescribeDiskConfigQuotaRequest()
	diskQuotaRequest.InquiryType = common.StringPtr("INQUIRY_CBS_CONFIG")
	diskQuotaRequest.Zones = common.StringPtrs([]string{zone})
	diskQuotaRequest.DiskChargeType = common.StringPtr(paymode)
	diskQuotaResp, err := ctrl.cbsClient.DescribeDiskConfigQuota(diskQuotaRequest)
	if err != nil {
		metrics.YunApiRequestTotal.WithLabelValues(DriverName, string(util.CBS), string(util.DescribeDiskQuota), "", util.GetTencentSdkErrCode(err)).Inc()
		metrics.CbsPvcsRequestTotal.WithLabelValues(DriverName, string(util.Provision), util.GetTencentSdkErrCode(err)).Inc()
		return "", err
	}
	metrics.YunApiRequestTotal.WithLabelValues(DriverName, string(util.CBS), string(util.DescribeDiskQuota), "", string(util.Success)).Inc()
	metrics.YunApiRequestCostSeconds.WithLabelValues(DriverName, string(util.CBS), string(util.DescribeDiskQuota)).Observe(time.Since(sTime).Seconds())

	if inputDiskType == "cbs" {
		return getDiskTypeForDefaultStorageClass(diskSize, diskQuotaResp)
	}

	return verifyDiskTypeIsSupported(inputDiskType, diskSize, diskQuotaResp)
}

func (ctrl *cbsController) describeDisks(diskIds []*string, instanceId string, action util.PvcAction) (*cbs.DescribeDisksResponse, error) {
	describeDiskRequest := cbs.NewDescribeDisksRequest()
	describeDiskRequest.DiskIds = diskIds
	sTime := time.Now()
	describeDiskResponse, err := ctrl.cbsClient.DescribeDisks(describeDiskRequest)
	if err != nil {
		metrics.YunApiRequestTotal.WithLabelValues(DriverName, string(util.CBS), string(util.DescribeDisks), "", util.GetTencentSdkErrCode(err)).Inc()
		switch action {
		case util.Provision, util.Delete:
			metrics.CbsPvcsRequestTotal.WithLabelValues(DriverName, string(action), util.GetTencentSdkErrCode(err)).Inc()
		case util.Attach, util.Detach:
			metrics.OperationErrorsTotal.WithLabelValues(DriverName, string(action), *diskIds[0], instanceId, util.GetTencentSdkErrCode(err)).Inc()
		default:
		}
		return nil, err
	}
	metrics.YunApiRequestTotal.WithLabelValues(DriverName, string(util.CBS), string(util.DescribeDisks), "", string(util.Success)).Inc()
	metrics.YunApiRequestCostSeconds.WithLabelValues(DriverName, string(util.CBS), string(util.DescribeDisks)).Observe(time.Since(sTime).Seconds())

	return describeDiskResponse, nil
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

func getDiskTypeForDefaultStorageClass(diskSize uint64, diskQuotaResp *cbs.DescribeDiskConfigQuotaResponse) (string, error) {
	createType := ""
	var minDiskSize, maxDiskSize uint64

	for _, diskConfig := range diskQuotaResp.Response.DiskConfigSet {
		if *diskConfig.Available && *diskConfig.DiskType == DiskTypeCloudPremium {
			createType = DiskTypeCloudPremium
			minDiskSize = *diskConfig.MinDiskSize
			maxDiskSize = *diskConfig.MaxDiskSize
		}
		if *diskConfig.Available && *diskConfig.DiskType == DiskTypeCloudBasic {
			createType = DiskTypeCloudBasic
			minDiskSize = *diskConfig.MinDiskSize
			maxDiskSize = *diskConfig.MaxDiskSize
			break
		}
	}

	if createType != "" && (diskSize < minDiskSize || diskSize > maxDiskSize) {
		return createType, fmt.Errorf("disk size is invalid. Must in [%d, %d]", minDiskSize, maxDiskSize)
	}

	return createType, nil
}

func verifyDiskTypeIsSupported(inputDiskType string, diskSize uint64, diskQuotaResp *cbs.DescribeDiskConfigQuotaResponse) (string, error) {
	for _, diskConfig := range diskQuotaResp.Response.DiskConfigSet {
		if *diskConfig.Available && *diskConfig.DiskType == inputDiskType {
			if diskSize < *diskConfig.MinDiskSize || diskSize > *diskConfig.MaxDiskSize {
				return inputDiskType, fmt.Errorf("disk size is invalid. Must in [%d, %d]", *diskConfig.MinDiskSize, *diskConfig.MaxDiskSize)
			}

			return inputDiskType, nil
		}
	}

	return "", nil
}

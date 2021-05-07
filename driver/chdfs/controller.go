package chdfs

import (
	"context"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	chdfs "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/chdfs/v20190718"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var controllerCaps = []csi.ControllerServiceCapability_RPC_Type{
	csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
}

type controllerServer struct {
	chdfsClient *chdfs.Client
	*csicommon.DefaultControllerServer
}

func newControllerServer(driver *csicommon.CSIDriver, chdfsURL, region, secretID, secertKey string) (csi.ControllerServer, error) {
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = chdfsURL
	client, err := chdfs.NewClient(common.NewCredential(secretID, secertKey), region, cpf)
	if err != nil {
		glog.Error("Error in new chdfs client: ", err)
		return nil, err
	}

	return &controllerServer{
		chdfsClient:             client,
		DefaultControllerServer: csicommon.NewDefaultControllerServer(driver),
	}, nil
}

func (ctrl *controllerServer) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
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

func (ctrl *controllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	glog.Infof("ControllerExpandVolume: ControllerExpandVolumeRequest is %v", *req)

	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	capacityRange := req.GetCapacityRange()
	if capacityRange == nil {
		return nil, status.Error(codes.InvalidArgument, "Capacity range not provided")
	}

	newCHDFSSizeBytes := capacityRange.GetRequiredBytes()

	diskId := req.VolumeId
	//check size
	chdfsInfoReq := chdfs.NewDescribeFileSystemRequest()
	chdfsInfoReq.FileSystemId = &diskId

	chdfsInfoResp, err := ctrl.chdfsClient.DescribeFileSystem(chdfsInfoReq)
	if err != nil {
		glog.Error("Error in DescribeFileSystem request: ", err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	glog.Infof("old chdfs size: %v", *chdfsInfoResp.Response.FileSystem.CapacityQuota)

	if *chdfsInfoResp.Response.FileSystem.CapacityQuota == uint64(newCHDFSSizeBytes) {
		glog.Infof("Request size %v is equal *disk.DiskSize %v", newCHDFSSizeBytes, *chdfsInfoResp.Response.FileSystem.CapacityQuota)
		return &csi.ControllerExpandVolumeResponse{
			CapacityBytes:         int64(*chdfsInfoResp.Response.FileSystem.CapacityQuota),
			NodeExpansionRequired: false,
		}, nil
	}

	//expand chdfs
	resizeRequest := chdfs.NewModifyFileSystemRequest()

	glog.Infof("disid: %v, quota: %v", diskId, newCHDFSSizeBytes)

	resizeRequest.FileSystemId = common.StringPtr(diskId)
	resizeRequest.CapacityQuota = common.Uint64Ptr(uint64(newCHDFSSizeBytes))

	_, err = ctrl.chdfsClient.ModifyFileSystem(resizeRequest)
	if err != nil {
		glog.Error("Error in ModifyFileSystem request: ", err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	ticker := time.NewTicker(time.Second * 3)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*120)
	defer cancel()

	for {
		select {
		case <-ticker.C:
			chdfsInfo, err := ctrl.chdfsClient.DescribeFileSystem(chdfsInfoReq)
			if err != nil {
				glog.Infof("DescribeFileSystem CHDFS failed: %v", err)
				continue
			}
			if *chdfsInfo.Response.FileSystem.CapacityQuota == uint64(newCHDFSSizeBytes) {
				return &csi.ControllerExpandVolumeResponse{
					CapacityBytes:         newCHDFSSizeBytes,
					NodeExpansionRequired: true,
				}, nil
			}
		case <-ctx.Done():
			return nil, status.Errorf(codes.DeadlineExceeded, "CHDFS disk %s is not expanded before deadline exceeded", diskId)
		}
	}
}

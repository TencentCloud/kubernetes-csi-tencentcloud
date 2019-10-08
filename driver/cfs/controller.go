package cfs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/util"
	cfsv3 "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cfs/v20190719"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	v3common "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	CFSDefaultStorageType  = "SD"
	CFSDefaultPGroupID     = "pgroupbasic"
	CFSDefaultNetInterface = "vpc"
	CFSDefaultProtocol     = "NFS"
	CFSStateCreating       = "creating"
	CFSStateAvailble       = "available"
	GB                     = 1 << (10 * 3)
)

type controllerServer struct {
	*csicommon.DefaultControllerServer
	cfsClient *cfsv3.Client
	zone      string
}

// CreateVolume provisions an cfs file
func (cs *controllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	glog.Infof("CreateVolume CreateVolumeRequest is %v:", req)

	volumeCapabilities := req.GetVolumeCapabilities()
	name := req.GetName()
	if len(name) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume Name must be provided")
	}
	if len(volumeCapabilities) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume Volume capabilities must be provided")
	}

	capacityBytes := req.GetCapacityRange().GetRequiredBytes()
	volSizeBytes := int64(capacityBytes)
	requestGiB := int(util.RoundUpGiB(volSizeBytes))

	parameters := req.GetParameters()

	glog.Infof("req.name is :  %s   ", req.Name)

	var zone, storageType, pgroupID, vpcID, subnetID string

	for k, v := range parameters {
		switch strings.ToLower(k) {
		case "zone":
			zone = v
		case "storagetype":
			storageType = v
		case "pgroupid":
			pgroupID = v
		case "vpcid":
			vpcID = v
		case "subnetid":
			subnetID = v
		}
	}

	request := cfsv3.NewCreateCfsFileSystemRequest()

	if storageType != "" {
		request.StorageType = common.StringPtr(storageType)
	} else {
		request.StorageType = common.StringPtr(CFSDefaultStorageType)
	}

	if pgroupID != "" {
		request.PGroupId = common.StringPtr(pgroupID)
	} else {
		request.PGroupId = common.StringPtr(CFSDefaultPGroupID)
	}

	request.NetInterface = common.StringPtr(CFSDefaultNetInterface)

	if zone != "" {
		request.Zone = common.StringPtr(zone)
	} else {
		request.Zone = common.StringPtr(cs.zone)
	}

	request.FsName = common.StringPtr(name)
	request.Protocol = common.StringPtr(CFSDefaultProtocol)

	if vpcID == "" {
		return nil, status.Error(codes.InvalidArgument, "VpcID should not nil")

	}
	request.VpcId = common.StringPtr(vpcID)
	if subnetID == "" {
		return nil, status.Error(codes.InvalidArgument, "subnetID should not nil")
	}
	request.SubnetId = common.StringPtr(subnetID)

	updateCfsClent(cs.cfsClient)
	response, err := cs.cfsClient.CreateCfsFileSystem(request)

	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	cfsID := response.Response.FileSystemId

	if cfsID == nil {
		return nil, status.Error(codes.Internal, "CreateCfsFileSystem's Response FileSystemId is nil!")
	}

	ticker := time.NewTicker(time.Second * 3)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*180)
	defer cancel()

	for {
		select {
		case <-ticker.C:
			descReq := cfsv3.NewDescribeCfsFileSystemsRequest()
			descReq.FileSystemId = cfsID

			descResp, err := cs.cfsClient.DescribeCfsFileSystems(descReq)
			if err != nil {
				glog.Warningf("DescribeCfsFileSystems %v failed: %v", *cfsID, err)
				continue
			}
			if descResp.Response != nil && len(descResp.Response.FileSystems) >= 1 {
				for _, f := range descResp.Response.FileSystems {
					if *f.FileSystemId == *cfsID && f.LifeCycleState != nil {
						if *f.LifeCycleState == CFSStateAvailble {

							// describe mount point
							mountTargetReq := cfsv3.NewDescribeMountTargetsRequest()
							mountTargetReq.FileSystemId = cfsID

							mountTargetResp, err := cs.cfsClient.DescribeMountTargets(mountTargetReq)
							if err != nil {
								glog.Warningf("DescribeMountTargets %v failed: %v", *cfsID, err)
								continue
							}
							if mountTargetResp.Response != nil && len(mountTargetResp.Response.MountTargets) >= 1 {
								for _, m := range mountTargetResp.Response.MountTargets {
									if m != nil && m.FileSystemId != nil && *m.FileSystemId == *cfsID {
										if m.IpAddress != nil {
											parameters["host"] = *m.IpAddress
										}
										if m.FSID != nil {
											parameters["fsid"] = *m.FSID
										}
										return &csi.CreateVolumeResponse{
											Volume: &csi.Volume{
												VolumeId:      *cfsID,
												CapacityBytes: int64(requestGiB * GB),
												VolumeContext: parameters,
											},
										}, nil
									}
								}
							}
						}
					}
				}
			}

		case <-ctx.Done():
			return nil, status.Error(codes.DeadlineExceeded, fmt.Sprintf("cfs %v is not ready before deadline exceeded", *cfsID))
		}
	}

}

// DeleteVolume delete an cfs file
func (cs *controllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	glog.Infof("DeleteVolume DeleteVolumeRequest is %v:", req)

	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}

	if err := cs.Driver.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid delete volume request: %v", req)
	}

	request := cfsv3.NewDeleteCfsFileSystemRequest()
	request.FileSystemId = common.StringPtr(req.VolumeId)
	updateCfsClent(cs.cfsClient)
	_, err := cs.cfsClient.DeleteCfsFileSystem(request)

	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.DeleteVolumeResponse{}, nil
}

func updateCfsClent(client *cfsv3.Client) *cfsv3.Client {
	secretID, secretKey, token, isTokenUpdate := GetSercet()
	if token != "" && isTokenUpdate {
		cred := v3common.Credential{
			SecretId:  secretID,
			SecretKey: secretKey,
			Token:     token,
		}
		client.WithCredential(&cred)
	}
	return client
}

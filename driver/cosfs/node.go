/*
 Copyright 2019 Tencent.

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

package cos

import (
	"context"
	"net"
	"net/http"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	SocketPath         = "/etc/csi-cos/cosfs.sock"
	MounterCosfs       = "cosfs"
	MounterGoosefsLite = "goosefs-lite"
	defaultDBGLevel    = "err"
)

type nodeServer struct {
	*csicommon.DefaultNodeServer
	launcherClient http.Client
}

type cosfsOptions struct {
	URL            string
	Bucket         string
	Path           string
	Mounter        string
	DebugLevel     string
	AdditionalArgs string
	CoreSite       string
	GoosefsLite    string
}

func NewNodeServer(driver *csicommon.CSIDriver) csi.NodeServer {
	return &nodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(driver),
		launcherClient: http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", SocketPath)
				},
			},
		},
	}
}

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	if err := validateNodePublishVolumeRequest(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "validate NodePublishVolumeRequest failed, %v", err)
	}

	volID := req.GetVolumeId()
	targetPath := req.GetTargetPath()

	options, err := parseCosfsOptions(req.GetVolumeContext())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "parse options failed, %v", err)
	}

	isMnt, err := createMountPoint(volID, targetPath, ns.launcherClient)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create targetPath %s failed, %v", targetPath, err)
	}
	if isMnt {
		glog.Infof("volume %s is already mounted to %s, skipping", volID, targetPath)
		return &csi.NodePublishVolumeResponse{}, nil
	}

	podUid := getPodUidFromTargetPath(targetPath)
	if podUid == "" {
		return nil, status.Errorf(codes.InvalidArgument, "getPodUidFromTargetPath failed, invalid targetPath %s", targetPath)
	}

	switch options.Mounter {
	case MounterCosfs:
		// Extract the tmp credential info from NodeStageSecrets and store to a unique tmp file.
		credFilePath, err := createCosfsCredentialFile(volID, options.Bucket, podUid, req.GetSecrets())
		if err != nil {
			return nil, status.Errorf(codes.Internal, "create cosfs credentialFile %s failed, %v", targetPath, err)
		}
		if err := mount(createCosfsMountCmd(targetPath, credFilePath, options), ns.launcherClient); err != nil {
			return nil, status.Errorf(codes.Internal, "mounter: %s, mount %s to %s failed, %v", MounterCosfs, volID, targetPath, err)
		}
	case MounterGoosefsLite:
		coreSiteXmlPath, goosefsLitePropertiesPath, err := createGoosefsLiteConfigFiles(podUid, options.URL, options.CoreSite, options.GoosefsLite, req.GetSecrets())
		if err != nil {
			return nil, status.Errorf(codes.Internal, "create goosefs-lite configFiles failed, %v", err)
		}
		if err := mount(createGoosefsLiteMountCmd(targetPath, coreSiteXmlPath, goosefsLitePropertiesPath, options), ns.launcherClient); err != nil {
			return nil, status.Errorf(codes.Internal, "mounter: %s, mount %s to %s failed, %v", MounterGoosefsLite, volID, targetPath, err)
		}
	}

	glog.Infof("successfully mounted volume %s to %s", volID, targetPath)

	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	if err := validateNodeUnpublishVolumeRequest(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "validate NodeUnpublishVolumeRequest failed, %v", err)
	}

	volID := req.GetVolumeId()
	targetPath := req.GetTargetPath()

	if err := umount(targetPath, ns.launcherClient); err != nil {
		return nil, status.Errorf(codes.Internal, "umount %s for volume %s failed, %v", targetPath, volID, err)
	}

	glog.Infof("Successfully unmounted volume %s from %s", req.GetVolumeId(), targetPath)

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "NodeExpandVolume is not implemented yet")
}

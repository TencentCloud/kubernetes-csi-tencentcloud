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

package cfs

import (
	"github.com/container-storage-interface/spec/lib/go/csi/v0"
	"github.com/golang/glog"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	cfsv3 "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cfs/v20190719"
	v3common "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	v3profile "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	"k8s.io/kubernetes/pkg/util/mount"
)

type driver struct {
	csiDriver *csicommon.CSIDriver
	endpoint  string

	ids *csicommon.DefaultIdentityServer
	ns  *nodeServer

	cap   []*csi.VolumeCapability_AccessMode
	cscap []*csi.ControllerServiceCapability

	region string
	zone   string
	cfsUrl string
}

const (
	DriverName     = "com.tencent.cloud.csi.cfs"
	DriverVerision = "0.3.0"
)

func NewDriver(nodeID, endpoint, region, zone, cfsUrl string) *driver {
	glog.Infof("Driver: %v version: %v", DriverName, DriverVerision)

	d := &driver{}

	d.endpoint = endpoint
	d.cfsUrl = cfsUrl
	d.region = region
	d.zone = zone

	csiDriver := csicommon.NewCSIDriver(DriverName, DriverVerision, nodeID)
	csiDriver.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
	})

	// CFS plugin does not support ControllerServiceCapability now.
	// If support is added, it should set to appropriate
	// ControllerServiceCapability RPC types.
	csiDriver.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME})

	d.csiDriver = csiDriver

	return d
}

func NewNodeServer(d *driver, mounter mount.Interface) *nodeServer {
	return &nodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d.csiDriver),
		mounter:           mounter,
	}
}

func NewControllerServer(d *driver) *controllerServer {
	secretID, secretKey, token, _ := GetSercet()

	cred := v3common.Credential{
		SecretId:  secretID,
		SecretKey: secretKey,
		Token:     token,
	}

	cpf := v3profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = d.cfsUrl

	cfsClient, _ := cfsv3.NewClient(&cred, d.region, cpf)
	return &controllerServer{
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d.csiDriver),
		cfsClient:               cfsClient,
		zone:                    d.zone,
	}
}

func (d *driver) Run() {
	s := csicommon.NewNonBlockingGRPCServer()
	s.Start(d.endpoint,
		csicommon.NewDefaultIdentityServer(d.csiDriver),
		NewControllerServer(d),
		NewNodeServer(d, mount.New("")))
	s.Wait()
}

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
	"net/http"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/dbdd4us/qcloudapi-sdk-go/metadata"
	"github.com/golang/glog"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/util"
	cfsv3 "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cfs/v20190719"
	v3common "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	v3profile "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	"k8s.io/utils/mount"
)

const (
	DriverName          = "com.tencent.cloud.csi.cfs"
	DriverVersion       = "v1.0.0"
	componentController = "controller"
	componentNode       = "node"

	Url     = "cfs.internal.tencentcloudapi.com"
	TestUrl = "cfs.test.tencentcloudapi.com"
)

type driver struct {
	csiDriver *csicommon.CSIDriver

	endpoint        string
	region          string
	zone            string
	cfsUrl          string
	componentType   string
	environmentType string
}

func NewDriver(nodeID, endpoint, region, zone, cfsUrl, componentType, environmentType string) *driver {
	glog.Infof("Driver: %v version: %v", DriverName, DriverVersion)

	csiDriver := csicommon.NewCSIDriver(DriverName, DriverVersion, nodeID)
	csiDriver.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
	})
	csiDriver.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
	})

	return &driver{
		csiDriver:       csiDriver,
		endpoint:        endpoint,
		cfsUrl:          cfsUrl,
		zone:            zone,
		region:          region,
		componentType:   componentType,
		environmentType: environmentType,
	}
}

func NewNodeServer(d *driver) *nodeServer {
	return &nodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d.csiDriver),
		mounter:           mount.New(""),
	}
}

func NewControllerServer(d *driver) *controllerServer {
	secretID, secretKey, token, _ := util.GetSercet()
	cred := v3common.Credential{
		SecretId:  secretID,
		SecretKey: secretKey,
		Token:     token,
	}

	metadataClient := metadata.NewMetaData(http.DefaultClient)
	if d.region == "" {
		r, err := util.GetFromMetadata(metadataClient, metadata.REGION)
		if err != nil {
			glog.Fatal(err)
		}
		d.region = r
	}
	if d.zone == "" {
		z, err := util.GetFromMetadata(metadataClient, metadata.ZONE)
		if err != nil {
			glog.Fatal(err)
		}
		d.zone = z
	}

	cpf := v3profile.NewClientProfile()
	if d.cfsUrl != "" {
		cpf.HttpProfile.Endpoint = d.cfsUrl
	} else if d.environmentType == "test" {
		cpf.HttpProfile.Endpoint = TestUrl
	} else {
		cpf.HttpProfile.Endpoint = Url
	}

	cfsClient, _ := cfsv3.NewClient(&cred, d.region, cpf)

	return &controllerServer{
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d.csiDriver),
		cfsClient:               cfsClient,
		zone:                    d.zone,
	}
}

func (d *driver) Run() {
	s := csicommon.NewNonBlockingGRPCServer()
	var cs *controllerServer
	var ns *nodeServer

	glog.Infof("Specify component type: %s", d.componentType)
	switch d.componentType {
	case componentController:
		cs = NewControllerServer(d)
	case componentNode:
		ns = NewNodeServer(d)
	default:
		cs = NewControllerServer(d)
		ns = NewNodeServer(d)
	}

	s.Start(d.endpoint, csicommon.NewDefaultIdentityServer(d.csiDriver), cs, ns)
	s.Wait()
}

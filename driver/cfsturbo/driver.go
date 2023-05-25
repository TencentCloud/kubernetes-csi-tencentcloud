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

package cfsturbo

import (
	"os"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/util"
	"k8s.io/utils/mount"
)

type driver struct {
	csiDriver *csicommon.CSIDriver
	endpoint  string
}

const (
	ClusterId     = "CLUSTER_ID"
	DriverName    = "com.tencent.cloud.csi.cfsturbo"
	DriverVersion = "cfsturbo"
)

func NewDriver(nodeID, endpoint, componentType string) *driver {
	glog.Infof("Driver: %v version: %v region: %v", DriverName, DriverVersion, componentType)

	csiDriver := csicommon.NewCSIDriver(DriverName, DriverVersion, nodeID)
	csiDriver.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
	})
	csiDriver.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
	})
	if componentType == "node" { //只在ds中启动
		install := newInstaller()
		// start a Goroutine loop
		go install.loop()
	}

	return &driver{
		endpoint:  endpoint,
		csiDriver: csiDriver,
	}
}

func NewControllerServer(d *driver) *controllerServer {
	return &controllerServer{
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d.csiDriver),
		mounter:                 mount.New(""),
		clusterId:               os.Getenv(ClusterId),
	}
}

func NewNodeServer(d *driver) *nodeServer {
	return &nodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d.csiDriver),
		VolumeLocks:       util.NewVolumeLocks(),
		mounter:           mount.New(""),
	}
}

func (d *driver) Run() {
	s := csicommon.NewNonBlockingGRPCServer()
	s.Start(d.endpoint, csicommon.NewDefaultIdentityServer(d.csiDriver), NewControllerServer(d), NewNodeServer(d))
	s.Wait()
}

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
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/util"
	"k8s.io/utils/mount"
)

type driver struct {
	csiDriver *csicommon.CSIDriver
	endpoint  string

	ids *csicommon.DefaultIdentityServer
	ns  *nodeServer

	cap   []*csi.VolumeCapability_AccessMode
	cscap []*csi.ControllerServiceCapability
}

const (
	DriverName     = "com.tencent.cloud.csi.cfsturbo"
	DriverVerision = "1.2.1"
)

func NewDriver(nodeID, endpoint string) *driver {
	glog.Infof("Driver: %v version: %v", DriverName, DriverVerision)

	d := &driver{}
	d.endpoint = endpoint

	csiDriver := csicommon.NewCSIDriver(DriverName, DriverVerision, nodeID)
	csiDriver.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
	})

	d.csiDriver = csiDriver

	return d
}

func NewNodeServer(d *driver, mounter mount.Interface) *nodeServer {
	return &nodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d.csiDriver),
		VolumeLocks:       util.NewVolumeLocks(),
		mounter:           mounter,
	}
}

func NewControllerServer(d *driver) *controllerServer {
	return &controllerServer{
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d.csiDriver),
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

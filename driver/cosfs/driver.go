/*
 Copyright 2019 THL A29 Limited, a Tencent company.

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
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
)

const version = "v1.0.0"

type driver struct {
	csiDriver *csicommon.CSIDriver
	endpoint  string
}

// NewDriver creates a new CSI driver for COS.
func NewDriver(endpoint, driverName, nodeID string) *driver {
	glog.Infof("Driver: %v version: %v", driverName, version)

	csiDriver := csicommon.NewCSIDriver(driverName, version, nodeID)
	csiDriver.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
	})

	return &driver{
		csiDriver: csiDriver,
		endpoint:  endpoint,
	}
}

func NewNodeServer(driver *csicommon.CSIDriver) csi.NodeServer {
	return &nodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(driver),
		mounter:           newMounter(),
	}
}

func (d *driver) Start() {
	server := csicommon.NewNonBlockingGRPCServer()
	server.Start(d.endpoint, csicommon.NewDefaultIdentityServer(d.csiDriver), nil,
		NewNodeServer(d.csiDriver))
	server.Wait()
}

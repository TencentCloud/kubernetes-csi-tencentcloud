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
	"github.com/container-storage-interface/spec/lib/go/csi/v0"
	"github.com/kubernetes-csi/drivers/pkg/csi-common"
)

const version = "0.3.0"

// Driver is an abstract for CSI Driver.
type Driver interface {
	// Start starts the CSI driver.
	Start(endpoint string)
}

// NewDriver creates a new CSI driver for COS.
func NewDriver(driverName, nodeID string) Driver {
	csiDriver := csicommon.NewCSIDriver(driverName, version, nodeID)
	csiDriver.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
	})
	return &driver{
		nodeServer:       newNodeServer(csiDriver, newMounter()),
		identityServer:   csicommon.NewDefaultIdentityServer(csiDriver),
		controllerServer: csicommon.NewDefaultControllerServer(csiDriver),
	}
}

// driver is an implement for COS CSI driver.
type driver struct {
	nodeServer       csi.NodeServer
	identityServer   csi.IdentityServer
	controllerServer csi.ControllerServer
}

func (d *driver) Start(endpoint string) {
	server := csicommon.NewNonBlockingGRPCServer()
	server.Start(endpoint, d.identityServer, d.controllerServer, d.nodeServer)
	server.Wait()
}

package chdfs

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
)

const version = "0.0.1"

// Driver is an abstract for CHDFS Driver.
type Driver interface {
	// Start starts the CHDFS driver.
	Start(endpoint string)
}

// NewDriver creates a new CSI driver for CHDFS.
func NewDriver(driverName, nodeID, chdfsURL, region, secretID, secertKey string) (Driver, error) {
	csiDriver := csicommon.NewCSIDriver(driverName, version, nodeID)
	csiDriver.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
	})
	csiDriver.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME})

	controller, err := newControllerServer(csiDriver, chdfsURL, region, secretID, secertKey)
	if err != nil {
		return nil, err
	}

	return &driver{
		nodeServer:       newNodeServer(csiDriver, newMounter()),
		identityServer:   csicommon.NewDefaultIdentityServer(csiDriver),
		controllerServer: controller,
	}, nil
}

// driver is an implement for CHDFS CSI driver.
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

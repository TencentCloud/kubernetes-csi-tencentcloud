package chdfs

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
)

const version = "v1.0.0"

// Driver is an abstract for CHDFS Driver.
type Driver interface {
	// Start starts the CHDFS driver.
	Start(endpoint string)
}

// NewDriver creates a new CSI driver for CHDFS.
func NewDriver(driverName, nodeID string) (Driver, error) {
	d := &driver{}
	csiDriver := csicommon.NewCSIDriver(driverName, version, nodeID)
	d.identityServer = csicommon.NewDefaultIdentityServer(csiDriver)
	d.nodeServer = newNodeServer(csiDriver)
	return d, nil
}

// driver is an implement for CHDFS CSI driver.
type driver struct {
	nodeServer     csi.NodeServer
	identityServer csi.IdentityServer
}

func (d *driver) Start(endpoint string) {
	server := csicommon.NewNonBlockingGRPCServer()
	server.Start(endpoint, d.identityServer, nil, d.nodeServer)
	server.Wait()
}

package main

import (
	"flag"

	"github.com/golang/glog"
	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/cfs"
)

var (
	endpoint        = flag.String("endpoint", "unix://plugin/csi.sock", "CSI endpoint")
	region          = flag.String("region", "", "tencent cloud api region")
	zone            = flag.String("zone", "", "cvm instance region")
	cfsUrl          = flag.String("cfs_url", "", "cfs api domain")
	nodeID          = flag.String("nodeID", "", "node ID")
	componentType   = flag.String("component_type", "", "component type")
	environmentType = flag.String("environment_type", "", "environment type")
)

func main() {
	flag.Set("logtostderr", "true")
	flag.Parse()

	if *nodeID == "" {
		glog.Fatal("nodeID is empty")
	}

	drv := cfs.NewDriver(*nodeID, *endpoint, *region, *zone, *cfsUrl, *componentType, *environmentType)
	drv.Run()
}

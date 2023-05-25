package main

import (
	"flag"

	"github.com/golang/glog"
	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/cfsturbo"
)

var (
	endpoint      = flag.String("endpoint", "unix://plugin/csi.sock", "CSI endpoint")
	nodeID        = flag.String("nodeID", "", "node ID")
	componentType = flag.String("component_type", "", "component type")
)

func main() {
	flag.Set("logtostderr", "true")
	flag.Parse()

	if *nodeID == "" {
		glog.Fatal("nodeID is empty")
	}

	drv := cfsturbo.NewDriver(*nodeID, *endpoint, *componentType)
	drv.Run()
}

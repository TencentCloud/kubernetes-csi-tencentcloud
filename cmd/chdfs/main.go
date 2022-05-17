package main

import (
	"flag"
	
	"github.com/golang/glog"
	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/chdfs"
)

var (
	endpoint   = flag.String("endpoint", "unix://csi/csi.sock", "CSI endpoint")
	driverName = flag.String("driverName", "com.tencent.cloud.csi.chdfs", "name of the driver")
	nodeID     = flag.String("nodeID", "", "node id")
)

func main() {
	flag.Parse()

	if *nodeID == "" {
		glog.Fatal("nodeID is empty")
	}

	driver := chdfs.NewDriver(*endpoint, *driverName, *nodeID)
	driver.Start()
}

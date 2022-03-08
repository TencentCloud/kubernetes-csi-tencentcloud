package main

import (
	"flag"
	"net/http"

	"github.com/dbdd4us/qcloudapi-sdk-go/metadata"
	"github.com/golang/glog"
	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/chdfs"
	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/util"
)

var (
	endpoint   = flag.String("endpoint", "unix://csi/csi.sock", "CSI endpoint")
	driverName = flag.String("driverName", "com.tencent.cloud.csi.chdfs", "name of the driver")
	nodeID     = flag.String("nodeID", "", "node id")
)

func main() {
	flag.Parse()
	metadataClient := metadata.NewMetaData(http.DefaultClient)

	if *nodeID == "" {
		n, err := util.GetFromMetadata(metadataClient, metadata.INSTANCE_ID)
		if err != nil {
			glog.Fatal(err)
		}
		nodeID = &n
	}

	driver, err := chdfs.NewDriver(*driverName, *nodeID)
	if err != nil {
		glog.Fatal(err)
	}

	driver.Start(*endpoint)
}

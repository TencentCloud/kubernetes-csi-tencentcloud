package main

import (
	"flag"
	"fmt"
	"net/http"
	"net/url"

	"github.com/dbdd4us/qcloudapi-sdk-go/metadata"
	"github.com/golang/glog"

	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/cfs"
)

const (
	TENCENTCLOUD_API_SECRET_ID  = "TENCENTCLOUD_API_SECRET_ID"
	TENCENTCLOUD_API_SECRET_KEY = "TENCENTCLOUD_API_SECRET_KEY"
)

var (
	endpoint         = flag.String("endpoint", fmt.Sprintf("unix://plugin/csi.sock", cfs.DriverName), "CSI endpoint")
	region           = flag.String("region", "", "tencent cloud api region")
	zone             = flag.String("zone", "", "cvm instance region")
	cfsUrl           = flag.String("cfs_url", "cfs.internal.tencentcloudapi.com", "cfs api domain")
	nodeID           = flag.String("nodeID", "", "node ID")
	metadataEndpoint = flag.String("metadata_endpoint", "http://metadata.tencentyun.com/latest/meta-data", "metadata endpoint.")
)

func main() {
	flag.Set("logtostderr", "true")
	flag.Parse()

	metadataClient := metadata.NewMetaData(http.DefaultClient, *metadataEndpoint)

	if *region == "" {
		r, err := metadataClient.Region()
		if err != nil {
			glog.Fatal(err)
		}
		region = &r
	}
	if *zone == "" {
		z, err := metadataClient.Zone()
		if err != nil {
			glog.Fatal(err)
		}
		zone = &z
	}

	u, err := url.Parse(*endpoint)
	if err != nil {
		glog.Fatalf("parse endpoint err: %s", err.Error())
	}

	if u.Scheme != "unix" {
		glog.Fatal("only unix socket is supported currently")
	}

	if *nodeID == "" {
		n, err := metadataClient.InstanceID()
		if err != nil {
			glog.Fatal(err)
		}
		nodeID = &n
	}

	drv := cfs.NewDriver(*nodeID, *endpoint, *region, *zone, *cfsUrl)

	drv.Run()
}

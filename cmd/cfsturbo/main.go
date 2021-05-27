package main

import (
	"flag"
	"net/http"
	"net/url"

	"github.com/dbdd4us/qcloudapi-sdk-go/metadata"
	"github.com/golang/glog"
	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/cfsturbo"
)

const (
	TENCENTCLOUD_API_SECRET_ID  = "TENCENTCLOUD_API_SECRET_ID"
	TENCENTCLOUD_API_SECRET_KEY = "TENCENTCLOUD_API_SECRET_KEY"
)

var (
	endpoint = flag.String("endpoint", "unix://plugin/csi.sock", "CSI endpoint")
	region   = flag.String("region", "", "tencent cloud api region")
	zone     = flag.String("zone", "", "cvm instance region")
	cfsURL   = flag.String("cfs_url", "cfs.internal.tencentcloudapi.com", "cfs api domain")
	nodeID   = flag.String("nodeID", "", "node ID")
)

func main() {
	flag.Set("logtostderr", "true")
	flag.Parse()

	metadataClient := metadata.NewMetaData(http.DefaultClient)

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

	drv := cfsturbo.NewDriver(*nodeID, *endpoint, *region, *zone, *cfsURL)

	drv.Run()
}

package main

import (
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/dbdd4us/qcloudapi-sdk-go/metadata"
	"github.com/golang/glog"

	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/cbs"
	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/util"
)

const (
	TENCENTCLOUD_CBS_API_SECRET_ID  = "TENCENTCLOUD_CBS_API_SECRET_ID"
	TENCENTCLOUD_CBS_API_SECRET_KEY = "TENCENTCLOUD_CBS_API_SECRET_KEY"

	ClusterId = "CLUSTER_ID"
)

var (
	endpoint          = flag.String("endpoint", fmt.Sprintf("unix:///var/lib/kubelet/plugins/%s/csi.sock", cbs.DriverName), "CSI endpoint")
	region            = flag.String("region", "", "tencent cloud api region")
	zone              = flag.String("zone", "", "cvm instance region")
	cbsUrl            = flag.String("cbs_url", "cbs.internal.tencentcloudapi.com", "cbs api domain")
	volumeAttachLimit = flag.Int64("volume_attach_limit", -1, "Value for the maximum number of volumes attachable for all nodes. If the flag is not specified then the value is default 20.")
	metadataEndpoint  = flag.String("metadata_endpoint", "http://metadata.tencentyun.com/latest/meta-data", "metadata endpoint.")
)

func main() {
	flag.Parse()
	defer glog.Flush()

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

	cp := util.NewCachePersister()

	drv, err := cbs.NewDriver(*region, *zone, os.Getenv(ClusterId), *volumeAttachLimit, metadataClient)
	if err != nil {
		glog.Fatal(err)
	}

	if err := drv.Run(u, *cbsUrl, cp); err != nil {
		glog.Fatal(err)
	}

	return
}

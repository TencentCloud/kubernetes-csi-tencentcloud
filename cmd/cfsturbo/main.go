package main

import (
	"flag"
	"net/http"
	"net/url"

	"github.com/dbdd4us/qcloudapi-sdk-go/metadata"
	"github.com/golang/glog"
	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/cfsturbo"
)

var (
	endpoint = flag.String("endpoint", "unix://plugin/csi.sock", "CSI endpoint")
	cfsURL   = flag.String("cfs_url", "cfs.internal.tencentcloudapi.com", "cfs api domain")
	nodeID   = flag.String("nodeID", "", "node ID")
)

func main() {
	flag.Set("logtostderr", "true")
	flag.Parse()

	u, err := url.Parse(*endpoint)
	if err != nil {
		glog.Fatalf("parse endpoint err: %s", err.Error())
	}

	if u.Scheme != "unix" {
		glog.Fatal("only unix socket is supported currently")
	}

	if *nodeID == "" {
		metadataClient := metadata.NewMetaData(http.DefaultClient)
		n, err := metadataClient.InstanceID()
		if err != nil {
			glog.Fatal(err)
		}
		nodeID = &n
	}

	drv := cfsturbo.NewDriver(*nodeID, *endpoint, *cfsURL)

	drv.Run()
}

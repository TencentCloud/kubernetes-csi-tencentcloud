package main

import (
	"flag"
	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/chdfs"
	"net/http"
	"os"

	"github.com/dbdd4us/qcloudapi-sdk-go/metadata"
	"github.com/golang/glog"
)

const (
	TENCENTCLOUD_CBS_API_SECRET_ID  = "TENCENTCLOUD_CBS_API_SECRET_ID"
	TENCENTCLOUD_CBS_API_SECRET_KEY = "TENCENTCLOUD_CBS_API_SECRET_KEY"
)

var (
	endpoint   = flag.String("endpoint", "unix://csi/csi.sock", "CSI endpoint")
	driverName = flag.String("drivername", "com.tencent.cloud.csi.chdfs", "name of the driver")
	nodeID     = flag.String("nodeid", "", "node id")
	secretId   = flag.String("secret_id", "", "tencent cloud api secret id")
	secretKey  = flag.String("secret_key", "", "tencent cloud api secret key")
	region     = flag.String("region", "", "tencent cloud api region")
	chdfsURL   = flag.String("chdfs_url", "chdfs.tencentcloudapi.com", "chdfs api domain")
)

func main() {
	flag.Parse()
	metadataClient := metadata.NewMetaData(http.DefaultClient)

	if *nodeID == "" {
		n, err := metadataClient.InstanceID()
		if err != nil {
			glog.Fatal(err)
		}
		nodeID = &n
	}
	if *region == "" {
		r, err := metadataClient.Region()
		if err != nil {
			glog.Fatal(err)
		}
		region = &r
	}
	if *secretId == "" {
		if secretIdFromEnv := os.Getenv(TENCENTCLOUD_CBS_API_SECRET_ID); secretIdFromEnv != "" {
			secretId = &secretIdFromEnv
		}
	}
	if *secretKey == "" {
		if secretKeyFromEnv := os.Getenv(TENCENTCLOUD_CBS_API_SECRET_KEY); secretKeyFromEnv != "" {
			secretKey = &secretKeyFromEnv
		}
	}

	driver, err := chdfs.NewDriver(*driverName, *nodeID, *chdfsURL, *region, *secretId, *secretKey)
	if err != nil {
		glog.Fatal(err)
	}

	driver.Start(*endpoint)
}

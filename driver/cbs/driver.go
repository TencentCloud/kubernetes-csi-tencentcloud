package cbs

import (
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"

	"github.com/golang/glog"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/cbs/tags"
	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/metrics"
	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/util"
)

const (
	DriverName          = "com.tencent.cloud.csi.cbs"
	DriverVerision      = "1.0.0"
	TopologyZoneKey     = "topology." + DriverName + "/zone"
	componentController = "controller"
	componentNode       = "node"
)

type Driver struct {
	client        kubernetes.Interface
	metadataStore util.CachePersister

	endpoint string
	region   string
	zone     string
	nodeID   string
	cbsUrl   string
	// TKE cluster ID
	clusterId         string
	componentType     string
	environmentType   string
	volumeAttachLimit int64
}

func NewDriver(endpoint, region, zone, nodeID, cbsUrl, clusterId, componentType, environmentType string, volumeAttachLimit int64, client kubernetes.Interface) *Driver {
	glog.Infof("Driver: %v version: %v", DriverName, DriverVerision)

	return &Driver{
		client:            client,
		metadataStore:     util.NewCachePersister(),
		endpoint:          endpoint,
		region:            region,
		zone:              zone,
		nodeID:            nodeID,
		cbsUrl:            cbsUrl,
		clusterId:         clusterId,
		componentType:     componentType,
		environmentType:   environmentType,
		volumeAttachLimit: volumeAttachLimit,
	}
}

func (drv *Driver) Run(enableMetricsServer bool, metricPort int64, timeInterval int) {
	s := csicommon.NewNonBlockingGRPCServer()
	var cs *cbsController
	var ns *cbsNode

	glog.Infof("Specify component type: %s", drv.componentType)
	switch drv.componentType {
	case componentController:
		cs = newCbsController(drv)
	case componentNode:
		ns = newCbsNode(drv)
	default:
		cs = newCbsController(drv)
		ns = newCbsNode(drv)
	}

	if cs != nil {
		if err := cs.LoadExDataFromMetadataStore(); err != nil {
			glog.Fatalf("failed to load metadata from store, err %v\n", err)
		}
	}

	if enableMetricsServer {
		// expose driver metrics
		metrics.RegisterMetrics()
		http.Handle("/metrics", promhttp.Handler())
		address := fmt.Sprintf(":%d", metricPort)
		glog.Infof("Starting metrics server at %s\n", address)
		go wait.Forever(func() {
			err := http.ListenAndServe(address, nil)
			if err != nil {
				glog.Errorf("Failed to listen on %s: %v", address, err)
			}
		}, 5*time.Second)
	}

	// Sync the tags of cluster and disks
	if drv.componentType == componentController || os.Getenv("ADDRESS") != "" {
		go func() {
			for {
				rand.Seed(time.Now().UnixNano())
				n := rand.Intn(timeInterval)
				glog.Infof("Begin to sync the tags of cluster and disks after sleeping %d minutes...\n", n)
				time.Sleep(time.Duration(n) * time.Minute)
				tags.UpdateDisksTags(drv.client, cs.cbsClient, cs.cvmClient, cs.tagClient, drv.region, drv.clusterId)
			}
		}()
	}

	s.Start(drv.endpoint, newCbsIdentity(), cs, ns)
	s.Wait()
}

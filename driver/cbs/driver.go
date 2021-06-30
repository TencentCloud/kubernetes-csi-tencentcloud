package cbs

import (
	"context"
	"fmt"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/metrics"
	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/util"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/util/wait"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"time"
)

const (
	DriverName      = "com.tencent.cloud.csi.cbs"
	DriverVerision  = "1.0.0"
	TopologyZoneKey = "topology." + DriverName + "/zone"
)

type Driver struct {
	region string
	zone   string
	// TKE cluster ID
	clusterId         string
	volumeAttachLimit int64
}

func NewDriver(region, zone, clusterId string, volumeAttachLimit int64) (*Driver, error) {
	driver := Driver{
		zone:              zone,
		region:            region,
		clusterId:         clusterId,
		volumeAttachLimit: volumeAttachLimit,
	}

	return &driver, nil
}

func (drv *Driver) Run(endpoint *url.URL, cbsUrl string, cachePersister util.CachePersister, enableMetricsServer bool, metricPort int64) error {
	controller, err := newCbsController(drv.region, drv.zone, cbsUrl, drv.clusterId, cachePersister)
	if err != nil {
		return err
	}

	if err := controller.LoadExDataFromMetadataStore(); err != nil {
		glog.Fatalf("failed to load metadata from store, err %v\n", err)
	}

	identity, err := newCbsIdentity()
	if err != nil {
		return err
	}

	node, err := newCbsNode(drv.region, drv.volumeAttachLimit)
	if err != nil {
		return err
	}

	logGRPC := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		glog.Infof("GRPC call: %s, request: %+v", info.FullMethod, req)
		resp, err := handler(ctx, req)
		if err != nil {
			glog.Errorf("GRPC error: %v", err)
		} else {
			glog.Infof("GRPC error: %v, response: %+v", err, resp)
		}
		return resp, err
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

	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(logGRPC),
	}

	srv := grpc.NewServer(opts...)

	csi.RegisterControllerServer(srv, controller)
	csi.RegisterIdentityServer(srv, identity)
	csi.RegisterNodeServer(srv, node)

	if endpoint.Scheme == "unix" {
		sockPath := path.Join(endpoint.Host, endpoint.Path)
		if _, err := os.Stat(sockPath); err != nil {
			if !os.IsNotExist(err) {
				return err
			}
		} else {
			if err := os.Remove(sockPath); err != nil {
				return err
			}
		}
	}

	listener, err := net.Listen(endpoint.Scheme, path.Join(endpoint.Host, endpoint.Path))
	if err != nil {
		return err
	}

	return srv.Serve(listener)
}

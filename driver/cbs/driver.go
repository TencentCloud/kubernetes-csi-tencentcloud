package cbs

import (
	"context"
	"net"
	"net/url"
	"os"
	"path"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	"google.golang.org/grpc"

	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/util"
)

const (
	DriverName      = "com.tencent.cloud.csi.cbs"
	DriverVerision  = "1.0.0"
	TopologyZoneKey = "topology." + DriverName + "/zone"
)

type Driver struct {
	region    string
	zone      string
	secretId  string
	secretKey string
}

func NewDriver(region string, zone string, secretId string, secretKey string) (*Driver, error) {
	driver := Driver{
		zone:      zone,
		region:    region,
		secretId:  secretId,
		secretKey: secretKey,
	}

	return &driver, nil
}

func (drv *Driver) Run(endpoint *url.URL, cbsUrl string, cachePersister util.CachePersister) error {
	controller, err := newCbsController(drv.secretId, drv.secretKey, drv.region, drv.zone, cbsUrl, cachePersister)
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

	node, err := newCbsNode(drv.secretId, drv.secretKey, drv.region)
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

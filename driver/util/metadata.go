package util

import (
	"fmt"
	"strings"
	"time"

	"github.com/dbdd4us/qcloudapi-sdk-go/metadata"
	"github.com/golang/glog"
	"golang.org/x/net/context"
)

var m = map[string]func(*metadata.MetaData) (string, error){
	metadata.REGION:      (*metadata.MetaData).Region,
	metadata.ZONE:        (*metadata.MetaData).Zone,
	metadata.INSTANCE_ID: (*metadata.MetaData).InstanceID,
}

func GetFromMetadata(metadata *metadata.MetaData, k string) (string, error) {
	trimk := strings.TrimLeft(k, "placement/")
	ticker := time.NewTicker(time.Second * 2)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()

	for {
		select {
		case <-ticker.C:
			v, err := m[k](metadata)
			if err != nil {
				glog.Errorf("get %s from metadata failed, err: %v, will retry in 2 seconds", trimk, err)
				continue
			}
			if v == "" {
				glog.Errorf("get %s from metadata failed, the %s is empty, will retry in 2 seconds", trimk, trimk)
				continue
			}
			glog.Infof("%s: %s", trimk, v)
			return v, err
		case <-ctx.Done():
			return "", fmt.Errorf("get %s from metadata failed before deadline exceeded", trimk)
		}
	}
}

package cbs

import (
	"fmt"
	"sync"

	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/util"
	cbs "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cbs/v20170312"
	cvm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
)

type cbsSnapshot struct {
	SourceVolumeId string `json:"sourceVolumeId"`
	SnapName       string `json:"snapName"`
	SnapId         string `json:"sanpId"`
	CreatedAt      int64  `json:"createdAt"`
	SizeBytes      int64  `json:"sizeBytes"`
}

type cbsSnapshotsCache struct {
	mux             *sync.Mutex
	cbsSnapshotMaps map[string]*cbsSnapshot
}

func (cache *cbsSnapshotsCache) add(id string, cbsSnap *cbsSnapshot) {
	cache.mux.Lock()
	defer cache.mux.Unlock()

	cache.cbsSnapshotMaps[id] = cbsSnap
}

func (cache *cbsSnapshotsCache) delete(id string) {
	cache.mux.Lock()
	defer cache.mux.Unlock()

	delete(cache.cbsSnapshotMaps, id)
}

func getCbsSnapshotByName(snapName string) (*cbsSnapshot, error) {
	cbsSnapshotsMapsCache.mux.Lock()
	defer cbsSnapshotsMapsCache.mux.Unlock()

	for _, cbsSnap := range cbsSnapshotsMapsCache.cbsSnapshotMaps {
		if cbsSnap.SnapName == snapName {
			return cbsSnap, nil
		}
	}
	return nil, fmt.Errorf("snapshot name %s does not exit in the snapshots list", snapName)
}

func updateCbsClent(client *cbs.Client) *cbs.Client {
	secretID, secretKey, token, isTokenUpdate := util.GetSercet()
	if token != "" && isTokenUpdate {
		cred := common.Credential{
			SecretId:  secretID,
			SecretKey: secretKey,
			Token:     token,
		}
		client.WithCredential(&cred)
	}
	return client
}

func updateCvmClent(client *cvm.Client) *cvm.Client {
	secretID, secretKey, token, isTokenUpdate := util.GetSercet()
	if token != "" && isTokenUpdate {
		cred := common.Credential {
			SecretId:  secretID,
			SecretKey: secretKey,
			Token:     token,
		}
		client.WithCredential(&cred)
	}
	return client
}

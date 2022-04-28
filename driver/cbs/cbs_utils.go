package cbs

import (
	"fmt"
	"strings"
	"sync"

	cbs "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cbs/v20170312"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	cvm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
	tag "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/tag/v20180813"

	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/util"
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

func updateClient(cbsClient *cbs.Client, cvmClient *cvm.Client, tagclient *tag.Client) {
	secretID, secretKey, token, isTokenUpdate := util.GetSercet()
	if token != "" && isTokenUpdate {
		cred := common.Credential{
			SecretId:  secretID,
			SecretKey: secretKey,
			Token:     token,
		}
		cbsClient.WithCredential(&cred)
		cvmClient.WithCredential(&cred)
		tagclient.WithCredential(&cred)
	}
}

func convertToIntreeTargetPath(targetPath string) string {
	return strings.Replace(strings.TrimSuffix(targetPath, "/mount"), "~csi", "~qcloud-cbs", 1)
}

func convertToIntreeStagingPath(targetPath, volumeId string) string {
	return strings.Split(targetPath, "/pods/")[0] + "/plugins/kubernetes.io/qcloud-cbs/mounts/" + volumeId
}

package tags

import (
	"context"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/golang/glog"
	cbs "github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/cbs/v20170312"
	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/util"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	cvm "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/cvm/v20170312"
	tag "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/tag/v20180813"
)

const (
	TKESERVICETYPE    = "ccs"
	TKERESOURCEPREFIX = "cluster"
	DriverName        = "com.tencent.cloud.csi.cbs"
	ProvisionedBy     = "pv.kubernetes.io/provisioned-by"
)

// UpdateDisksTags 集群和云硬盘标签同步的入口
func UpdateDisksTags(client kubernetes.Interface, cbsClient *cbs.Client, cvmClient *cvm.Client, tagClient *tag.Client, region, clusterId string) {
	updateClient(cbsClient, cvmClient, tagClient)
	clusterTags, err := GetClusterTags(tagClient, region, clusterId)
	if err != nil {
		glog.Errorf("UpdateDisksTags GetClusterTags failed, err: %v", err)
		return
	}
	glog.Infof("UpdateDisksTags GetClusterTags success, clusterTags: %v", clusterTags)

	configMapTags, err := GetConfigTags()
	if err != nil {
		glog.Errorf("UpdateDisksTags GetConfigTags failed, err: %v", err)
		return
	}
	glog.Infof("UpdateDisksTags GetConfigTags success, configTags: %v", configMapTags)

	needReplaceTags, needDeleteTags := CompareTags(clusterTags, configMapTags)
	if len(needReplaceTags) != 0 || len(needDeleteTags) != 0 {
		glog.Infof("Begin to update disks' tags and cm, needReplaceTags: %v, needDeleteTags: %v", needReplaceTags, needDeleteTags)
		err = updateDisksTags(client, cbsClient, cvmClient, tagClient, region, clusterId, needReplaceTags, needDeleteTags)
		if err != nil {
			glog.Errorf("UpdateDisksTags updateDisksTags failed")
			return
		}
		err = UpdateConfigTags(clusterTags)
		if err != nil {
			glog.Errorf("UpdateDisksTags UpdateConfig failed, err: %v", err)
			return
		}
	}
	glog.Infof("UpdateDisksTags success")
}

// GetClusterTags 调用云标签 api 获取集群标签
func GetClusterTags(tagClient *tag.Client, region, clusterId string) (map[string]string, error) {
	tagRequest := tag.NewDescribeResourceTagsByResourceIdsRequest()
	tagRequest.ServiceType = common.StringPtr(TKESERVICETYPE)
	tagRequest.ResourcePrefix = common.StringPtr(TKERESOURCEPREFIX)
	tagRequest.ResourceIds = common.StringPtrs([]string{clusterId})
	tagRequest.ResourceRegion = common.StringPtr(region)
	tagRequest.Limit = common.Uint64Ptr(100)

	tagResp, err := tagClient.DescribeResourceTagsByResourceIds(tagRequest)
	if err != nil {
		return nil, err
	}

	clusterTags := make(map[string]string)
	for _, tag := range tagResp.Response.Tags {
		clusterTags[*tag.TagKey] = *tag.TagValue
	}

	return clusterTags, err
}

// CompareTags 比较集群和 cm 的标签，返回需要更新的部分
func CompareTags(clusterTags, configMapTags map[string]string) (map[string]string, map[string]string) {
	needReplaceTags := make(map[string]string)
	needDeleteTags := make(map[string]string)

	for csk, csv := range clusterTags {
		if cmv, ok := configMapTags[csk]; !ok {
			needReplaceTags[csk] = csv
		} else if cmv != csv {
			needReplaceTags[csk] = csv
		}
	}

	for cmk, cmv := range configMapTags {
		if _, ok := clusterTags[cmk]; !ok {
			needDeleteTags[cmk] = cmv
		}
	}

	return needReplaceTags, needDeleteTags
}

// updateDisksTags 更新标签云硬盘
func updateDisksTags(client kubernetes.Interface, cbsClient *cbs.Client, cvmClient *cvm.Client, tagClient *tag.Client, region, clusterId string, needReplaceTags, needDeleteTags map[string]string) error {
	disksCreatedByCbs, err := GetDisks(client)
	if err != nil {
		glog.Errorf("updateDisksTags GetDisks failed, err: %v", err)
		return err
	}
	if len(disksCreatedByCbs) == 0 {
		glog.Infof("updateDisksTags success, there are no disks in this cluster")
		return nil
	}
	glog.Infof("updateDisksTags GetDisks success, disksCreatedByCbs: %v", disksCreatedByCbs)

	uin, err := GetOwnerUin()
	if err != nil {
		glog.Errorf("updateDisksTags GetOwnerUin failed, err: %v", err)
		return err
	}
	glog.Infof("updateDisksTags GetOwnerUin success, uin: %v", uin)

	var wg sync.WaitGroup
	errNum := 0
	maxWorkerNum := 5
	ch := make(chan string, len(disksCreatedByCbs))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if len(disksCreatedByCbs) < 5 {
		maxWorkerNum = len(disksCreatedByCbs)
	}

	for i := 0; i < maxWorkerNum; i++ {
		go worker(cbsClient, cvmClient, tagClient, uin, region, needReplaceTags, needDeleteTags, ch, &errNum, &wg, ctx)
	}

	for _, diskId := range disksCreatedByCbs {
		wg.Add(1)
		ch <- diskId
	}

	wg.Wait()
	if errNum != 0 {
		glog.Errorf("updateDisksTags failed")
		return fmt.Errorf("updateDisksTags failed")
	}
	glog.Infof("updateDisksTags success")
	return nil
}

// GetDisks 获取所有由 cbs-provisioner 动态创建的云硬盘
func GetDisks(client kubernetes.Interface) (map[string]string, error) {
	ctx := context.Background()
	pvs, err := client.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	disksCreatedByCbs := make(map[string]string, 0)
	for _, pv := range pvs.Items {
		if isCreatedByCbs(pv) && pv.Spec.CSI != nil && pv.Spec.CSI.Driver == DriverName {
			disksCreatedByCbs[pv.Name] = pv.Spec.CSI.VolumeHandle
		}
	}

	return disksCreatedByCbs, nil
}

func worker(cbsClient *cbs.Client, cvmClient *cvm.Client, tagClient *tag.Client, uin int64, region string, needReplaceTags, needDeleteTags map[string]string, ch chan string, errNum *int, wg *sync.WaitGroup, ctx context.Context) {
	for {
		select {
		case diskId := <-ch:
			resource := fmt.Sprintf("qcs::cvm:%s:uin/%v:volume/%s", region, uin, diskId)
			glog.Infof("disk resource is: %s", resource)
			updateClient(cbsClient, cvmClient, tagClient)
			if err := ModifyCbsTags(tagClient, resource, needReplaceTags, needDeleteTags); err != nil {
				glog.Errorf("updateDisksTags failed, disk: %s, err: %v", diskId, err)
				*errNum++
				wg.Done()
				continue
			}

			glog.Infof("updateDisksTags success, disk: %s", diskId)
			time.Sleep(time.Duration(1) * time.Second)
			wg.Done()
		case <-ctx.Done():
			return
		}
	}
}

// ModifyCbsTags resource:"qcs::cvm:ap-beijing:uin/xxx:volume/disk-id"
func ModifyCbsTags(tagClient *tag.Client, resource string, needReplaceTags, needDeleteTags map[string]string) error {
	modifyTagReq := tag.NewModifyResourceTagsRequest()

	for k, v := range needReplaceTags {
		key := k
		value := v
		modifyTagReq.ReplaceTags = append(modifyTagReq.ReplaceTags, &tag.Tag{TagKey: &key, TagValue: &value})
	}

	for k := range needDeleteTags {
		key := k
		modifyTagReq.DeleteTags = append(modifyTagReq.DeleteTags, &tag.TagKeyObject{TagKey: &key})
	}

	modifyTagReq.Resource = &resource
	_, err := tagClient.ModifyResourceTags(modifyTagReq)
	if err != nil {
		return err
	}

	return nil
}

// updateClient 更新 client credential
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

// isCreatedByCbs 检测 pv 是否由 cbs-csi 创建
func isCreatedByCbs(pv corev1.PersistentVolume) bool {
	v, ok := pv.Annotations[ProvisionedBy]
	if !ok {
		return false
	}

	if v != DriverName {
		return false
	}

	return true
}

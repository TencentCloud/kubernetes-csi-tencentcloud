package cfs

import (
	"os"

	"github.com/golang/glog"
	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/cloud"
)

func GetSercet() (secretID, secretKey, token string, isTokenUpdate bool) {
	secretID = os.Getenv("TENCENTCLOUD_API_SECRET_ID")
	secretKey = os.Getenv("TENCENTCLOUD_API_SECRET_KEY")

	if secretID != "" && secretKey != "" {
		return
	}

	var err error
	if secretID == "" || secretKey == "" {
		glog.Info("Get secretID or secretKey from env failed, will use cloud norm!")
		secretID, secretKey, token, isTokenUpdate, err = GetNormTmpSecret()
		if err != nil {
			glog.Errorf("GetNormTmpSecret error %v", err)
			return "", "", "", false
		}
	}

	return
}

func GetNormTmpSecret() (string, string, string, bool, error) {
	return cloud.GetNormCredentialInstance().GetCredential()
}

package tags

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/bitly/go-simplejson"
	"github.com/golang/glog"
)

const (
	envClusterID        = "CLUSTER_ID"
	envAppID            = "APPID"
)

type NormResponseUin struct {
	Code     int     `json:"returnValue"`
	Msg      string  `json:"returnMsg"`
	Version  string  `json:"version"`
	Password string  `json:"password"`
	Data     UinData `json:"returnData"`
}

type UinData struct {
	Uin int64 `json:"uin"`
}

func GetOwnerUin() (int64, error) {
	normURL := getNormUrl()
	client := &http.Client{}

	req := map[string]interface{}{
		"eventId":   rand.Uint32(),
		"timestamp": time.Now().Unix(),
		"caller":    "cloudprovider",
		"callee":    "NORM",
		"version":   "1",
		"password":  "cloudprovider",
		"interface": map[string]interface{}{
			"interfaceName": "NORM.GetClusterUin",
			"para":          map[string]interface{}{},
		},
	}

	b, err := json.Marshal(req)
	if err != nil {
		return 0, err
	}

	b = setNormReqExt(b)

	hreq, err := http.NewRequest("POST", normURL, bytes.NewReader(b))
	if err != nil {
		return 0, err
	}

	hreq.Header.Set("Content-type", "application/json")

	hresp, err := client.Do(hreq)
	if err != nil {
		return 0, err
	}
	defer hresp.Body.Close()

	if hresp.StatusCode != 200 {
		return 0, errors.New("client.Do httpResponse.StatusCode != 200")
	}

	body, err := ioutil.ReadAll(hresp.Body)
	if err != nil {
		return 0, err
	}

	normResp := &NormResponseUin{}
	err = json.Unmarshal(body, normResp)
	if err != nil {
		return 0, err
	}
	glog.V(8).Infof("norm resp body is %v", *normResp)

	if normResp.Code != 0 {
		return 0, errors.New(normResp.Msg)
	}
	return normResp.Data.Uin, nil
}

func getNormUrl() string {
	url := os.Getenv("QCLOUD_NORM_URL")
	if url == "" {
		url = "http://169.254.0.40:80/norm/api"
	}
	return url
}

//从环境变量中获取clusterID和appId放入norm的para中 for meta cluster
func setNormReqExt(body []byte) []byte {
	js, err := simplejson.NewJson(body)
	if err != nil {
		glog.Error("SetNormReqExt NewJson error,", err)
		return nil
	}
	if os.Getenv(envClusterID) != "" {
		js.SetPath([]string{"interface", "para", "unClusterId"}, os.Getenv(envClusterID))
		glog.V(4).Info("SetNormReqExt set unClusterId", os.Getenv(envClusterID))

	}
	if os.Getenv(envAppID) != "" {
		js.SetPath([]string{"interface", "para", "appId"}, os.Getenv(envAppID))
		glog.V(4).Info("SetNormReqExt set appId", os.Getenv(envAppID))

	}

	out, err := js.Encode()
	if err != nil {
		glog.Error("SetNormReqExt Encode error,", err)
		return body
	}

	return out
}
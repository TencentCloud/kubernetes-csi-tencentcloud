package cloud

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/golang/glog"
)

const NormExpiredDuration = time.Second * 7200

var (
	refresher NormRefresher
	cred      NormCredential
	once      sync.Once
)

func GetNormCredentialInstance() *NormCredential {
	once.Do(func() {
		refresher, _ = NewNormRefresher(NormExpiredDuration)
		cred, _ = NewNormCredential(NormExpiredDuration, refresher)
	})
	return &cred
}

type NormResponse struct {
	Code     int            `json:"returnValue"`
	Msg      string         `json:"returnMsg"`
	Version  string         `json:"version"`
	Password string         `json:"password"`
	Data     CredentialData `json:"returnData"`
}

type CredentialData struct {
	Credentials struct {
		SessionToken string `json:"sessionToken"`
		TmpSecretId  string `json:"tmpSecretId"`
		TmpSecretKey string `json:"tmpSecretKey"`
	} `json:"credentials"`
	ExpiredTime int `json:"expiredTime"`
}

func getNormUrl() string {
	url := os.Getenv("QCLOUD_NORM_URL")
	if url == "" {
		url = "http://169.254.0.40:80/norm/api"
	}
	return url
}

func GetTkeCredential() (*CredentialData, error) {

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
			"interfaceName": "NORM.AssumeTkeCredential",
			"para":          map[string]interface{}{},
		},
	}

	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	hreq, err := http.NewRequest("POST", normURL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}

	hreq.Header.Set("Content-type", "application/json")

	hresp, err := client.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer hresp.Body.Close()

	if hresp.StatusCode != 200 {
		return nil, errors.New("client.Do httpResponse.StatusCode != 200")
	}

	body, err := ioutil.ReadAll(hresp.Body)
	if err != nil {
		return nil, err
	}

	normResp := &NormResponse{}
	err = json.Unmarshal(body, normResp)
	if err != nil {
		return nil, err
	}
	glog.V(8).Infof("norm resp body is %v", *normResp)

	if normResp.Code != 0 {
		return nil, errors.New(normResp.Msg)
	}
	return &normResp.Data, nil
}

type Refresher interface {
	Refresh() (string, string, string, int, error)
}

type NormRefresher struct {
	expiredDuration time.Duration
}

func NewNormRefresher(expiredDuration time.Duration) (NormRefresher, error) {
	return NormRefresher{
		expiredDuration: expiredDuration,
	}, nil
}

func (nr NormRefresher) Refresh() (secretId string, secretKey string, token string, expiredAt int, err error) {
	rsp, err := GetTkeCredential()
	if err != nil {
		return "", "", "", 0, err
	}
	secretId = rsp.Credentials.TmpSecretId
	secretKey = rsp.Credentials.TmpSecretKey
	token = rsp.Credentials.SessionToken
	expiredAt = rsp.ExpiredTime

	return
}

type NormCredential struct {
	secretId  string
	secretKey string
	token     string

	expiredAt       time.Time
	expiredDuration time.Duration // TODO: maybe confused with this? find a better way

	lock sync.Mutex

	refresher Refresher
}

func NewNormCredential(expiredDuration time.Duration, refresher Refresher) (NormCredential, error) {
	return NormCredential{
		expiredDuration: expiredDuration,
		refresher:       refresher,
	}, nil
}

func (normCred *NormCredential) GetSecretId() (string, error) {
	normCred.lock.Lock()
	defer normCred.lock.Unlock()
	if normCred.needRefresh() {
		if err := normCred.refresh(); err != nil {
			return "", err
		}
	}
	return normCred.secretId, nil
}

func (normCred *NormCredential) GetSecretKey() (string, error) {
	normCred.lock.Lock()
	defer normCred.lock.Unlock()
	if normCred.needRefresh() {
		if err := normCred.refresh(); err != nil {
			return "", err
		}
	}
	return normCred.secretKey, nil
}

func (normCred *NormCredential) GetCredential() (string, string, string, bool, error) {
	normCred.lock.Lock()
	defer normCred.lock.Unlock()
	isNeedRefresh := normCred.needRefresh()
	if isNeedRefresh {
		if err := normCred.refresh(); err != nil {
			return "", "", "", isNeedRefresh, err
		}
	}
	return normCred.secretId, normCred.secretKey, normCred.token, isNeedRefresh, nil
}

func (normCred *NormCredential) needRefresh() bool {
	return time.Now().Add(normCred.expiredDuration / time.Second / 2 * time.Second).After(normCred.expiredAt)
}

func (normCred *NormCredential) refresh() error {
	secretId, secretKey, token, expiredAt, err := normCred.refresher.Refresh()
	if err != nil {
		return err
	}

	normCred.updateExpiredAt(time.Unix(int64(expiredAt), 0))
	normCred.updateCredential(secretId, secretKey, token)

	return nil
}

func (normCred *NormCredential) updateExpiredAt(expiredAt time.Time) {
	normCred.expiredAt = expiredAt
}

func (normCred *NormCredential) updateCredential(secretId, secretKey, token string) {

	normCred.secretId = secretId
	normCred.secretKey = secretKey
	normCred.token = token
}

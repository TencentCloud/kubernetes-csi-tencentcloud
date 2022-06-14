package cos

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
	"github.com/spf13/viper"
)

const (
	// Used to create staging mount point password files.
	perm = 0600

	credentialID  = "SecretId"
	credentialKey = "SecretKey"
	cosUrlPrefix  = "cos."
	cosUrlSuffix  = ".myqcloud.com"

	cosfsPasswordFileDirectory    = "/etc/csi-cos/cosfs/"
	goosefsLiteConfigPath         = "/etc/csi-cos/goosefs-lite"
	coreSiteXmlFileName           = "core-site.xml"
	goosefsLitePropertiesFileName = "goosefs-lite.properties"
)

type Configuration struct {
	XMLName    xml.Name   `xml:"configuration"`
	Properties []Property `xml:"property"`
}

type Property struct {
	Name  string `xml:"name"`
	Value string `xml:"value"`
}

func createCosfsCredentialFile(volID, bucket, podUid string, secrets map[string]string) (string, error) {
	sid, skey, err := getSecretCredential(secrets)
	if err != nil {
		return "", fmt.Errorf("getSecretCredential failed, %v", err)
	}
	cosbucket := strings.TrimSpace(bucket)
	credential := strings.Join([]string{cosbucket, sid, skey}, ":")
	// compute password file sha256 and write is on password file name, so if file exist,
	// then we needn't create a new password file
	// file name like  testcos-123123123_fa51046944be10ef2d231dce44b3278414698678f9be0551a9299b15f75fecf1
	credSHA := sha256.New()
	credSHA.Write([]byte(credential))
	shaString := hex.EncodeToString(credSHA.Sum(nil))
	passwdFilename := fmt.Sprintf("%s%s/%s_%s", cosfsPasswordFileDirectory, podUid, bucket, shaString)

	glog.Infof("cosfs password file name is %s", passwdFilename)

	if _, err := os.Stat(passwdFilename); err != nil {
		if os.IsNotExist(err) {
			if err := WriteFile(passwdFilename, credential, perm); err != nil {
				return "", fmt.Errorf("create tmp password file for volume %s failed, error: %v", volID, err)
			}
		} else {
			return "", fmt.Errorf("stat password file failed, error: %v", err)
		}
	} else {
		glog.Infof("password file %s is exist, and sha256 is same", passwdFilename)
	}

	return passwdFilename, nil
}

func createGoosefsLiteConfigFiles(podUid, cosUrl, coreSite, goosefsLite string, secrets map[string]string) (string, string, error) {
	coreSiteXmlPath, err := createCoreSiteXml(podUid, cosUrl, coreSite, secrets)
	if err != nil {
		return "", "", fmt.Errorf("create core-site.xml failed, %v", err)
	}

	goosefsLitePropertiesPath, err := createGoosefsLiteProperties(podUid, goosefsLite)
	if err != nil {
		if err != nil {
			return "", "", fmt.Errorf("create goosefs-lite.properties failed, %v", err)
		}
	}

	return coreSiteXmlPath, goosefsLitePropertiesPath, nil
}

func createCoreSiteXml(podUid, cosUrl, coreSite string, secrets map[string]string) (string, error) {
	sid, skey, err := getSecretCredential(secrets)
	if err != nil {
		return "", fmt.Errorf("getSecretCredential failed, %v", err)
	}

	region, err := getRegionFromUrl(cosUrl)
	if err != nil {
		return "", fmt.Errorf("getRegionFromUrl failed, %v", err)
	}

	coreSiteXml := Configuration{
		Properties: []Property{
			{
				Name:  "fs.cosn.impl",
				Value: "org.apache.hadoop.fs.CosFileSystem",
			},
			{
				Name:  "fs.cosn.userinfo.secretId",
				Value: sid,
			},
			{
				Name:  "fs.cosn.userinfo.secretKey",
				Value: skey,
			},
			{
				Name:  "fs.cosn.bucket.region",
				Value: region,
			},
		},
	}

	noReadAheadQueueSize := true
	noUploadThreadPool := true
	if coreSite != "" {
		args := strings.Split(coreSite, ",")
		for _, arg := range args {
			a := strings.Split(strings.TrimSpace(arg), "=")
			if len(a) != 2 {
				return "", fmt.Errorf("error: invalid argument %s", arg)
			}
			if a[0] == "fs.cosn.read.ahead.queue.size" {
				noReadAheadQueueSize = false
			}
			if a[0] == "fs.cosn.upload_thread_pool" {
				noUploadThreadPool = false
			}
			coreSiteXml.Properties = append(coreSiteXml.Properties, Property{Name: a[0], Value: a[1]})
		}
	}

	if noReadAheadQueueSize {
		coreSiteXml.Properties = append(coreSiteXml.Properties, Property{Name: "fs.cosn.read.ahead.queue.size", Value: "16"})
	}
	if noUploadThreadPool {
		coreSiteXml.Properties = append(coreSiteXml.Properties, Property{Name: "fs.cosn.upload_thread_pool", Value: "32"})
	}

	coreSiteXmlb, err := xml.MarshalIndent(coreSiteXml, "", "  ")
	if err != nil {
		return "", fmt.Errorf("xml.MarshalIndent failed, error: %v", err)
	}

	coreSiteXmlPath := strings.Join([]string{goosefsLiteConfigPath, podUid, coreSiteXmlFileName}, "/")
	err = WriteFile(coreSiteXmlPath, string(coreSiteXmlb), 0644)
	if err != nil {
		return "", fmt.Errorf("WriteFile failed, error: %v", err)
	}

	return coreSiteXmlPath, nil
}

func createGoosefsLiteProperties(podUid, goosefsLite string) (string, error) {
	viper.SetConfigType("properties")

	if goosefsLite != "" {
		args := strings.Split(goosefsLite, ",")
		for _, arg := range args {
			a := strings.Split(strings.TrimSpace(arg), "=")
			if len(a) != 2 {
				return "", fmt.Errorf("error: invalid argument %s", arg)
			}
			viper.Set(a[0], a[1])
		}
	}

	goosefsLitePropertiesPath := strings.Join([]string{goosefsLiteConfigPath, podUid, goosefsLitePropertiesFileName}, "/")
	err := os.MkdirAll(path.Dir(goosefsLitePropertiesPath), os.ModePerm)
	if err != nil {
		return "", fmt.Errorf("mkdir %s failed, error: %v", path.Dir(goosefsLitePropertiesPath), err)
	}
	err = viper.WriteConfigAs(goosefsLitePropertiesPath)
	if err != nil {
		return "", fmt.Errorf("viper.WriteConfigAs failed, error: %v", err)
	}

	return goosefsLitePropertiesPath, nil
}

func getPodUidFromTargetPath(targetPath string) string {
	// /var/lib/kubelet/pods/b53365d4-6171-4c77-9d28-c753733ee535/volumes/kubernetes.io~csi/goosefs-lite/mount
	dirs := strings.Split(targetPath, "/")
	for k, v := range dirs {
		if v == "pods" {
			return dirs[k+1]
		}
	}
	return ""
}

func getSecretCredential(secrets map[string]string) (string, string, error) {
	for k := range secrets {
		if k != credentialID && k != credentialKey {
			return "", "", fmt.Errorf("error: secret must contains %v or %v", credentialID, credentialKey)
		}
	}
	sid := strings.TrimSpace(secrets[credentialID])
	skey := strings.TrimSpace(secrets[credentialKey])
	return sid, skey, nil
}

func getRegionFromUrl(cosUrl string) (string, error) {
	u, err := url.Parse(cosUrl)
	if err != nil {
		return "", fmt.Errorf("parse the cos url %s failed, error: %v", cosUrl, err)
	}

	if strings.HasPrefix(u.Host, cosUrlPrefix) && strings.HasSuffix(u.Host, cosUrlSuffix) {
		return strings.Split(u.Host, ".")[1], nil
	}

	return "", fmt.Errorf("error: the cos url %s is invalid. ", cosUrl)
}

func WriteFile(file string, context string, fileMode fs.FileMode) error {
	err := os.MkdirAll(path.Dir(file), os.ModePerm)
	if err != nil {
		return err
	}

	writer, err := os.OpenFile(filepath.Clean(file), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fileMode)
	if err != nil {
		return err
	}
	defer writer.Close()

	_, err = fmt.Fprintln(writer, context)
	return err
}

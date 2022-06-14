package cos

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/spf13/viper"
)

func TestCreateCredentialFile(t *testing.T) {
	testCases := []struct {
		name              string
		bucket            string
		secrets           map[string]string
		expectFileContent string
		expectFileName    string
		expectError       bool
	}{
		{
			name:              "fakebucket passwd file is not exist and success",
			bucket:            "fakebucket",
			secrets:           map[string]string{"SecretId": "fakesid", "SecretKey": "fakeskey"},
			expectFileContent: "fakebucket:fakesid:fakeskey",
			expectFileName:    "/etc/csi-cos/fakebucket_ab652776e8b7080c53a40360e61f4ca77fb3cf10b3bf51d5a3190a31c366c9a3",
			expectError:       false,
		},
		{
			name:              "fakebucket passwd file is exist and success",
			bucket:            "fakebucket",
			secrets:           map[string]string{"SecretId": "fakesid", "SecretKey": "fakeskey"},
			expectFileContent: "fakebucket:fakesid:fakeskey",
			expectFileName:    "/etc/csi-cos/fakebucket_ab652776e8b7080c53a40360e61f4ca77fb3cf10b3bf51d5a3190a31c366c9a3",
			expectError:       false,
		},
		{
			name:              "fakebucket passwd file and write another bucket sid skey success",
			bucket:            "fakebucket2",
			secrets:           map[string]string{"SecretId": "fakesid2", "SecretKey": "fakeskey2"},
			expectFileContent: "fakebucket2:fakesid2:fakeskey2",
			expectFileName:    "/etc/csi-cos/fakebucket2_bbbe2c05487bef063f303207c87e5819242092d24716a45a0fcd382ccda34472",
			expectError:       false,
		},
		{
			name:              "fakebucket passwd file exist and sid skey have space charactor",
			bucket:            "fakebucket2",
			secrets:           map[string]string{"SecretId": "fakesid2 ", "SecretKey": "fakeskey2\n"},
			expectFileContent: "fakebucket2:fakesid2:fakeskey2",
			expectFileName:    "/etc/csi-cos/fakebucket2_bbbe2c05487bef063f303207c87e5819242092d24716a45a0fcd382ccda34472",
			expectError:       false,
		},
		{
			name:              "fakebucket passwd file exist and sid skey changed",
			bucket:            "fakebucket",
			secrets:           map[string]string{"SecretId": "fakesid22 ", "SecretKey": "fakeskey22\n"},
			expectFileContent: "fakebucket:fakesid22:fakeskey22",
			expectFileName:    "/etc/csi-cos/fakebucket_117b0f6b735a4c489b493caed277e4002f69b037390047f8801642a6c8b28dc8",
			expectError:       false,
		},
		{
			name:              "secret is not valid fail",
			bucket:            "fakebucket23",
			secrets:           map[string]string{"SecretId111": "fakesid22", "SecretKey111": "fakeskey22"},
			expectFileContent: "fakebucket2:fakesid22:fakeskey22",
			expectFileName:    "/etc/csi-cos/fakebucket2_bbbe2c05487bef063f303207c87e5819242092d24716a45a0fcd382ccda34472",
			expectError:       true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			filepath, err := createCosfsCredentialFile("testvol", tc.bucket, tc.secrets)
			if err != nil && !tc.expectError {
				t.Fatalf("find error %v", err)
			}
			if err != nil {
				t.Log("error occur is in expected return")
				return
			}
			if _, err := os.Stat(filepath); err != nil {
				t.Fatalf("find error %v", err)
			}
			if filepath != tc.expectFileName {
				t.Fatalf("filepath %s is not equal expectFileName %s", filepath, tc.expectFileName)
			}
			data, err := ioutil.ReadFile(filepath)
			if err != nil {
				t.Fatalf("read  fakePassword file  error %v", err)
			}
			if string(data) != tc.expectFileContent {
				t.Fatal("password file data is not equal   expectFileContent")
			}
		})
	}
	if err := os.Remove("/etc/csi-cos/fakebucket_ab652776e8b7080c53a40360e61f4ca77fb3cf10b3bf51d5a3190a31c366c9a3"); err != nil {
		t.Fatalf("Remove password file error %v", err)
	}
	if err := os.Remove("/etc/csi-cos/fakebucket2_bbbe2c05487bef063f303207c87e5819242092d24716a45a0fcd382ccda34472"); err != nil {
		t.Fatalf("Remove password file error %v", err)
	}
	if err := os.Remove("/etc/csi-cos/fakebucket_117b0f6b735a4c489b493caed277e4002f69b037390047f8801642a6c8b28dc8"); err != nil {
		t.Fatalf("Remove password file error %v", err)
	}

}

var coreSiteXml = `<configuration>
  <property>
    <name>fs.cosn.impl</name>
    <value>org.apache.hadoop.fs.CosFileSystem</value>
  </property>
  <property>
    <name>fs.cosn.userinfo.secretId</name>
    <value>******</value>
  </property>
  <property>
    <name>fs.cosn.userinfo.secretKey</name>
    <value>******</value>
  </property>
  <property>
    <name>fs.cosn.bucket.region</name>
    <value>ap-guangzhou</value>
  </property>
  <property>
    <name>fs.cosn.read.ahead.queue.size</name>
    <value>16</value>
  </property>
  <property>
    <name>fs.cosn.upload_thread_pool</name>
    <value>32</value>
  </property>
</configuration>`

func TestXml(t *testing.T) {
	coreSite := Configuration{}
	err := xml.Unmarshal([]byte(coreSiteXml), &coreSite)
	if err != nil {
		fmt.Printf("err: %v\n", err)
		return
	}
	fmt.Printf("coreSite: %+v\n", coreSite)

	coreSite = Configuration{
		Properties: []Property{
			{
				Name:  "aaa",
				Value: "aaa",
			},
			{
				Name:  "bbb",
				Value: "bbb",
			},
		},
	}
	coreSiteb, err := xml.MarshalIndent(coreSite, "", "  ")
	if err != nil {
		fmt.Printf("err: %v\n", err)
		return
	}
	fmt.Printf("coreSiteb:\n%s\n", string(coreSiteb))
}

var goosefsLiteProperties = `goosefs.fuse.list.entries.cache.enabled=true
goosefs.fuse.list.entries.cache.max.size=100000
goosefs.fuse.list.entries.cache.max.expiration.time=15000
goosefs.fuse.async.release.wait_time.max.ms=5000
goosefs.fuse.umount.timeout=120000`

func TestProperties(t *testing.T) {
	viper.SetConfigType("properties")
	if err := viper.ReadConfig(bytes.NewBuffer([]byte(goosefsLiteProperties))); err != nil {
		fmt.Printf("err: %v\n", err)
		return
	}
	fmt.Printf("goosefs.fuse.list.entries.cache.enabled: %v\n", viper.Get("goosefs.fuse.list.entries.cache.enabled"))
	fmt.Printf("goosefs.fuse.list.entries.cache.max.expiration.time: %v\n", viper.Get("goosefs.fuse.list.entries.cache.max.expiration.time"))
	fmt.Printf("goosefs.fuse.umount.timeout: %v\n", viper.Get("goosefs.fuse.umount.timeout"))

	viper.Set("goosefs.fuse.list.entries.cache.max.size", "1111")
	viper.Set("goosefs.fuse.list.entries.cache.max.expiration.time", "2222")
	viper.Set("goosefs.fuse.async.release.wait_time.max.ms", "3333")
	if err := viper.WriteConfigAs("config.properties"); err != nil {
		fmt.Printf("err: %v\n", err)
		return
	}
	configProperties, err := os.ReadFile("config.properties")
	if err != nil {
		fmt.Printf("err: %v\n", err)
		return
	}
	fmt.Printf("\nconfig.properties:\n%s\n", string(configProperties))
	if err := os.Remove("config.properties"); err != nil {
		fmt.Printf("err: %v\n", err)
		return
	}
}

func TestGetRegionFromUrl(t *testing.T) {
	region, err := getRegionFromUrl("http://cos.ap-guangzhou.myqcloud.com")
	if err != nil {
		fmt.Printf("err: %v\n", err)
	}
	fmt.Printf("region: %s\n", region)
}

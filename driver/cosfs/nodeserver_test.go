package cos

import (
	"io/ioutil"
	"os"
	"testing"

	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	"k8s.io/kubernetes/pkg/util/mount"
)

func newFakeMounter() *mount.FakeMounter {
	return &mount.FakeMounter{
		MountPoints: []mount.MountPoint{},
		Log:         []mount.FakeAction{},
	}
}

func newFakeSafeFormatAndMounter(fakeMounter *mount.FakeMounter) mount.SafeFormatAndMount {
	return mount.SafeFormatAndMount{
		Interface: fakeMounter,
		Exec:      mount.NewFakeExec(nil),
	}

}

func newFakeNode() *nodeServer {
	return &nodeServer{
		mounter:           newMounter(),
		DefaultNodeServer: csicommon.NewDefaultNodeServer(csicommon.NewCSIDriver("cos-csiplugin", version, "fakenode")),
	}
}

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
			expectFileName:    "/tmp/fakebucket_ab652776e8b7080c53a40360e61f4ca77fb3cf10b3bf51d5a3190a31c366c9a3",
			expectError:       false,
		},
		{
			name:              "fakebucket passwd file is exist and success",
			bucket:            "fakebucket",
			secrets:           map[string]string{"SecretId": "fakesid", "SecretKey": "fakeskey"},
			expectFileContent: "fakebucket:fakesid:fakeskey",
			expectFileName:    "/tmp/fakebucket_ab652776e8b7080c53a40360e61f4ca77fb3cf10b3bf51d5a3190a31c366c9a3",
			expectError:       false,
		},
		{
			name:              "fakebucket passwd file and write another bucket sid skey success",
			bucket:            "fakebucket2",
			secrets:           map[string]string{"SecretId": "fakesid2", "SecretKey": "fakeskey2"},
			expectFileContent: "fakebucket2:fakesid2:fakeskey2",
			expectFileName:    "/tmp/fakebucket2_bbbe2c05487bef063f303207c87e5819242092d24716a45a0fcd382ccda34472",
			expectError:       false,
		},
		{
			name:              "fakebucket passwd file exist and sid skey have space charactor",
			bucket:            "fakebucket2",
			secrets:           map[string]string{"SecretId": "fakesid2 ", "SecretKey": "fakeskey2\n"},
			expectFileContent: "fakebucket2:fakesid2:fakeskey2",
			expectFileName:    "/tmp/fakebucket2_bbbe2c05487bef063f303207c87e5819242092d24716a45a0fcd382ccda34472",
			expectError:       false,
		},
		{
			name:              "fakebucket passwd file exist and sid skey changed",
			bucket:            "fakebucket",
			secrets:           map[string]string{"SecretId": "fakesid22 ", "SecretKey": "fakeskey22\n"},
			expectFileContent: "fakebucket:fakesid22:fakeskey22",
			expectFileName:    "/tmp/fakebucket_117b0f6b735a4c489b493caed277e4002f69b037390047f8801642a6c8b28dc8",
			expectError:       false,
		},
		{
			name:              "secret is not valid fail",
			bucket:            "fakebucket23",
			secrets:           map[string]string{"SecretId111": "fakesid22", "SecretKey111": "fakeskey22"},
			expectFileContent: "fakebucket2:fakesid22:fakeskey22",
			expectFileName:    "/tmp/fakebucket2_bbbe2c05487bef063f303207c87e5819242092d24716a45a0fcd382ccda34472",
			expectError:       true,
		},
	}
	fns := newFakeNode()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			filepath, err := fns.createCredentialFile("testvol", tc.bucket, tc.secrets)
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
	if err := os.Remove("/tmp/fakebucket_ab652776e8b7080c53a40360e61f4ca77fb3cf10b3bf51d5a3190a31c366c9a3"); err != nil {
		t.Fatalf("Remove password file error %v", err)
	}
	if err := os.Remove("/tmp/fakebucket2_bbbe2c05487bef063f303207c87e5819242092d24716a45a0fcd382ccda34472"); err != nil {
		t.Fatalf("Remove password file error %v", err)
	}
	if err := os.Remove("/tmp/fakebucket_117b0f6b735a4c489b493caed277e4002f69b037390047f8801642a6c8b28dc8"); err != nil {
		t.Fatalf("Remove password file error %v", err)
	}

}

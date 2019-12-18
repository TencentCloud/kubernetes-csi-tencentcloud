/*
 Copyright 2019 THL A29 Limited, a Tencent company.

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

 http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package cos

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// Used to create staging mount point password files.
	perm = 0600

	defaultDBGLevel          = "info"
	cosPasswordFile          = "/etc/passwd-cosfs"
	cosPasswordFileDirectory = "/tmp/"
	secretKey                = "sec"
	credentialID             = "SecretId"
	credentialKey            = "SecretKey"
	SocketPath               = "/tmp/cosfs.sock"
)

func newNodeServer(driver *csicommon.CSIDriver, mounter mounter) csi.NodeServer {
	return &nodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(driver),
		mounter:           mounter,
	}
}

type nodeServer struct {
	*csicommon.DefaultNodeServer
	mounter mounter
}

type cosfsOptions struct {
	URL            string
	Bucket         string
	DebugLevel     string
	AdditionalArgs string
}

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	if err := validateNodePublishVolumeRequest(req); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	volID := req.GetVolumeId()
	targetPath := req.GetTargetPath()

	options, err := parseCosfsOptions(req.GetVolumeContext())
	if err != nil {
		glog.Errorf("parse options from VolumeAttributes for %s failed: %v", volID, err)
		return nil, status.Errorf(codes.InvalidArgument, "parse options failed: %v", err)
	}

	isMnt, err := ns.createMountPoint(volID, targetPath)
	if err != nil {
		return nil, err
	}
	if isMnt {
		glog.Infof("Volume %s is already mounted to %s, skipping", volID, targetPath)
		return &csi.NodePublishVolumeResponse{}, nil
	}

	// Extract the tmp credential info from NodeStageSecrets and store to a unique tmp file.
	credFilePath, err := ns.createCredentialFile(volID, options.Bucket, req.GetSecrets())
	if err != nil {
		return nil, err
	}

	if err := ns.mounter.Mount(options, targetPath, credFilePath); err != nil {
		glog.Errorf("Mount %s to %s failed: %v", volID, targetPath, err)
		return nil, status.Errorf(codes.Internal, "mount failed: %v", err)
	}

	glog.Infof("successfully mounted volume %s to %s", volID, targetPath)

	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	if err := validateNodeUnpublishVolumeRequest(req); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	volID := req.GetVolumeId()
	targetPath := req.GetTargetPath()

	// use launcher
	if err := ns.mounter.Umount(targetPath); err != nil {
		glog.Errorf("Failed to umount point %s for volume %s: %v", targetPath, volID, err)
		return nil, status.Errorf(codes.Internal, "umount failed: %v", err)
	}

	glog.Infof("Successfully unmounted volume %s from %s", req.GetVolumeId(), targetPath)

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (ns *nodeServer) createMountPoint(volID, targetPath string) (bool, error) {
	glog.Infof("Creating staging mount point at %s for volume %s", targetPath, volID)
	return launcherCreateMountPoint(targetPath)
}

func parseCosfsOptions(attributes map[string]string) (*cosfsOptions, error) {
	options := &cosfsOptions{DebugLevel: defaultDBGLevel}
	for k, v := range attributes {
		switch strings.ToLower(k) {
		case "url":
			options.URL = v
		case "bucket":
			options.Bucket = v
		case "dbglevel":
			options.DebugLevel = v
		case "additional_args":
			options.AdditionalArgs = v
		}
	}
	return options, validateCosfsOptions(options)
}

func validateCosfsOptions(options *cosfsOptions) error {
	if options.URL == "" {
		return errors.New("COS service URL can't be empty")
	}
	if options.Bucket == "" {
		return errors.New("COS bucket can't be empty")
	}
	return nil
}

func (ns *nodeServer) createCredentialFile(volID, bucket string, secrets map[string]string) (string, error) {
	credential, err := getSecretCredential(bucket, secrets)
	if err != nil {
		glog.Errorf("getSecretCredential info from NodeStageSecrets failed: %v", err)
		return "", status.Errorf(codes.InvalidArgument, "get credential failed: %v", err)
	}

	// compute password file sha256 and write is on password file name, so if file exist,
	// then we needn't create a new password file
	// file name like  testcos-123123123_fa51046944be10ef2d231dce44b3278414698678f9be0551a9299b15f75fecf1
	credSHA := sha256.New()
	credSHA.Write([]byte(credential))
	shaString := string(hex.EncodeToString(credSHA.Sum(nil)))
	passwdFilename := fmt.Sprintf("%s%s_%s", cosPasswordFileDirectory, bucket, shaString)

	glog.Infof("cosfs password file name is %s", passwdFilename)

	if _, err := os.Stat(passwdFilename); err != nil {
		if os.IsNotExist(err) {
			if err := ioutil.WriteFile(passwdFilename, []byte(credential), perm); err != nil {
				glog.Errorf("create password file for volume %s failed: %v", volID, err)
				return "", status.Errorf(codes.Internal, "create tmp password file failed: %v", err)
			}
		} else {
			glog.Errorf("stat password file  %s failed: %v", passwdFilename, err)
			return "", status.Errorf(codes.Internal, "stat password file failed: %v", err)
		}
	} else {
		glog.Infof("password file %s is exist, and sha256 is same", passwdFilename)
	}

	return passwdFilename, nil
}

func getSecretCredential(bucket string, secrets map[string]string) (string, error) {
	for k := range secrets {
		if k != credentialID && k != credentialKey {
			return "", fmt.Errorf("secret must contains %v or %v", credentialID, credentialKey)
		}
	}
	sid := strings.TrimSpace(secrets[credentialID])
	skey := strings.TrimSpace(secrets[credentialKey])
	cosbucket := strings.TrimSpace(bucket)
	return strings.Join([]string{cosbucket, sid, skey}, ":"), nil
}

func launcherCreateMountPoint(targetPath string) (bool, error) {
	httpClient := http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", SocketPath)
			},
		},
	}

	body := make(map[string]string)
	body["stagingPath"] = targetPath
	bodyJson, _ := json.Marshal(body)
	response, err := httpClient.Post("http://unix/mount", "application/json", strings.NewReader(string(bodyJson)))
	if err != nil {
		return false, err
	}

	defer response.Body.Close()
	respBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return false, err
	}

	if response.StatusCode != http.StatusOK {
		return false, fmt.Errorf("the response of launcher mount is: %v", string(respBody))
	}

	var result map[string]string
	if err := json.Unmarshal(respBody, &result); err != nil {
		glog.Errorf("Unmarshal the response body of unix/mount failed. %v", err)
		return false, err
	}

	isMounted, err := strconv.ParseBool(result["isMounted"])
	if err != nil {
		glog.Errorf("parse the value of `isMounted` from string to boolean failed. %v", err)
		return false, err
	}

	return isMounted, nil
}

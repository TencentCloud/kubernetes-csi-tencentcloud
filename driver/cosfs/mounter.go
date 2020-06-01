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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/golang/glog"
	"k8s.io/utils/mount"
)

type mounter interface {
	IsMountPoint(p string) (bool, error)
	CreateMountPoint(point string) error
	Mount(options *cosfsOptions, mountPoint string, credentialFilePath string) error
	BindMount(from, to string, readOnly bool) error
	Umount(point string) error
	RemoveMountPoint(point string) error
}

func newMounter() mounter {
	return &defaultMounter{mounter: mount.New("")}
}

type defaultMounter struct {
	mounter mount.Interface
}

func (*defaultMounter) Mount(options *cosfsOptions, mountPoint string, credentialFilePath string) error {
	if err := sendCmdToLauncher(options, mountPoint, credentialFilePath); err != nil {
		return fmt.Errorf("failed to send mount command to launcher: %v", err)
	}

	return nil
}

func (*defaultMounter) BindMount(from, to string, readOnly bool) error {
	if err := execCmd("mount", "--bind", from, to); err != nil {
		return fmt.Errorf("failed to bind-mount %s to %s: %v", from, to, err)
	}
	if readOnly {
		if err := execCmd("mount", "-o", "remount,ro,bind", to); err != nil {
			return fmt.Errorf("failed read-only remount of %s: %v", to, err)
		}
	}
	return nil
}

func (m *defaultMounter) IsMountPoint(p string) (bool, error) {
	notMnt, err := m.mounter.IsLikelyNotMountPoint(p)
	if err != nil {
		return false, err
	}
	return !notMnt, nil
}

func (*defaultMounter) CreateMountPoint(point string) error {
	return os.MkdirAll(point, perm)
}

func (*defaultMounter) Umount(point string) error {
	httpClient := http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", SocketPath)
			},
		},
	}

	args := []string{
		"-l",
		point,
	}

	body := make(map[string]string)
	body["command"] = fmt.Sprintf("umount %s", strings.Join(args, " "))
	bodyJson, _ := json.Marshal(body)
	response, err := httpClient.Post("http://unix/launcher", "application/json", strings.NewReader(string(bodyJson)))
	if err != nil {
		return err
	}

	defer response.Body.Close()
	respBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("the response of launcher(action: umount) is: %v", string(respBody))
	}

	glog.Info("send umount command to launcher successfully")

	return nil
}

func (*defaultMounter) RemoveMountPoint(point string) error {
	return os.Remove(point)
}

func execCmd(cmd string, args ...string) error {
	glog.V(5).Infof("Exec command: %s %v", cmd, args)
	output, err := exec.Command(cmd, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("command %s failed: output %s, error: %v", cmd, string(output), err)
	}
	glog.V(4).Infof("command %s %v finished: %s", cmd, args, string(output))
	return nil
}

func sendCmdToLauncher(options *cosfsOptions, mountPoint string, credentialFilePath string) error {
	httpClient := http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", SocketPath)
			},
		},
	}

	bucketOrWithSubDir := options.Bucket
	if options.Path != "" {
		bucketOrWithSubDir = fmt.Sprintf("%s:%s", options.Bucket, options.Path)
	}
	args := []string{
		bucketOrWithSubDir,
		mountPoint,
		"-ourl=" + options.URL,
		"-odbglevel=" + options.DebugLevel,
		"-opasswd_file=" + credentialFilePath,
	}
	if options.AdditionalArgs != "" {
		args = append(args, options.AdditionalArgs)
	}

	body := make(map[string]string)
	body["command"] = fmt.Sprintf("cosfs %s", strings.Join(args, " "))
	bodyJson, _ := json.Marshal(body)
	response, err := httpClient.Post("http://unix/launcher", "application/json", strings.NewReader(string(bodyJson)))
	if err != nil {
		return err
	}

	defer response.Body.Close()
	respBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("the response of launcher(action: cosfs) is: %v", string(respBody))
	}

	glog.Info("send cosfs command to launcher successfully")

	return nil
}

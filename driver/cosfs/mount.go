/*
 Copyright 2019 Tencent.

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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/golang/glog"
)

func createMountPoint(volID, targetPath string, launcherClient http.Client) (bool, error) {
	glog.Infof("Creating staging mount point at %s for volume %s", targetPath, volID)

	body := make(map[string]string)
	body["targetPath"] = targetPath
	bodyJson, _ := json.Marshal(body)
	response, err := launcherClient.Post("http://unix/create", "application/json", strings.NewReader(string(bodyJson)))
	if err != nil {
		return false, err
	}

	defer response.Body.Close()
	respBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return false, fmt.Errorf("read response body failed, error: %v", err)
	}

	if response.StatusCode != http.StatusOK {
		return false, fmt.Errorf("error: the response of launcher create is: %v", string(respBody))
	}

	var result map[string]string
	if err := json.Unmarshal(respBody, &result); err != nil {
		return false, fmt.Errorf("unmarshal the response body of unix/create failed, error: %v", err)
	}

	isMounted, err := strconv.ParseBool(result["isMounted"])
	if err != nil {
		return false, fmt.Errorf("parse the value of `isMounted` from string to boolean failed, error: %v", err)
	}

	return isMounted, nil
}

func mount(cmd map[string]string, launcherClient http.Client) error {
	glog.Info("Sending mount command to launcher")

	bodyJson, _ := json.Marshal(cmd)
	response, err := launcherClient.Post("http://unix/mount", "application/json", strings.NewReader(string(bodyJson)))
	if err != nil {
		return err
	}

	defer response.Body.Close()
	respBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("read response body failed, error: %v", err)
	}

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("error: the response of launcher mount is: %v", string(respBody))
	}

	return nil
}

func createCosfsMountCmd(mountPoint, credentialFilePath string, options *cosfsOptions) map[string]string {
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

	cmd := make(map[string]string)
	cmd["command"] = fmt.Sprintf("cosfs %s", strings.Join(args, " "))

	return cmd
}

func createGoosefsLiteMountCmd(mountPoint, coreSiteXmlPath, goosefsLitePropertiesPath string, options *cosfsOptions) map[string]string {
	bucketOrWithSubDir := "cosn://" + options.Bucket
	if options.Path != "" {
		bucketOrWithSubDir = fmt.Sprintf("%s%s", bucketOrWithSubDir, options.Path)
	}

	args := []string{
		"-c " + coreSiteXmlPath,
		"-g " + goosefsLitePropertiesPath,
		mountPoint,
		bucketOrWithSubDir,
	}

	cmd := make(map[string]string)
	cmd["command"] = fmt.Sprintf("/goosefs-lite/bin/goosefs-lite mount %s", strings.Join(args, " "))

	return cmd
}

func umount(mountPoint string, launcherClient http.Client) error {
	args := []string{
		"-l",
		mountPoint,
	}

	body := make(map[string]string)
	body["command"] = fmt.Sprintf("umount %s", strings.Join(args, " "))
	bodyJson, _ := json.Marshal(body)
	response, err := launcherClient.Post("http://unix/umount", "application/json", strings.NewReader(string(bodyJson)))
	if err != nil {
		return err
	}

	defer response.Body.Close()
	respBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("read response body failed, error: %v", err)
	}

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("error: the response of launcher umount is: %v", string(respBody))
	}

	glog.Info("send umount command to launcher successfully")

	return nil
}

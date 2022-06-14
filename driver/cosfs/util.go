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
	"errors"
	"fmt"
	"runtime"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
)

func validateNodePublishVolumeRequest(req *csi.NodePublishVolumeRequest) error {
	if req.GetVolumeCapability() == nil {
		return errors.New("error: volume capability missing in request")
	}

	if req.GetVolumeId() == "" {
		return errors.New("error: volume ID missing in request")
	}

	if req.GetTargetPath() == "" {
		return errors.New("error: target path missing in request")
	}

	return nil
}

func validateNodeUnpublishVolumeRequest(req *csi.NodeUnpublishVolumeRequest) error {
	if req.GetVolumeId() == "" {
		return errors.New("error: volume ID missing in request")
	}

	if req.GetTargetPath() == "" {
		return errors.New("error: target path missing in request")
	}

	return nil
}

func parseCosfsOptions(attributes map[string]string) (*cosfsOptions, error) {
	options := &cosfsOptions{}
	for k, v := range attributes {
		switch strings.ToLower(k) {
		case "url":
			options.URL = v
		case "bucket":
			options.Bucket = v
		case "path":
			options.Path = v
		case "mounter":
			options.Mounter = v
		case "dbglevel":
			options.DebugLevel = v
		case "additional_args":
			options.AdditionalArgs = v
		case "core_site":
			options.CoreSite = v
		case "goosefs_lite":
			options.GoosefsLite = v
		}
	}

	if options.Mounter == "" {
		options.Mounter = MounterCosfs
	}

	if options.Mounter == MounterCosfs && options.DebugLevel == "" {
		options.DebugLevel = defaultDBGLevel
	}

	return options, validateCosfsOptions(options)
}

func validateCosfsOptions(options *cosfsOptions) error {
	if options.URL == "" {
		return errors.New("error: cos service URL can't be empty")
	}

	if options.Bucket == "" {
		return errors.New("error: cos bucket can't be empty")
	}

	if options.Mounter != MounterCosfs && options.Mounter != MounterGoosefsLite {
		return fmt.Errorf("error: mounter %s is not supported", options.Mounter)
	}

	if options.Mounter == MounterGoosefsLite && runtime.GOARCH != "amd64" {
		return fmt.Errorf("error: mounter %s does not support %s", options.Mounter, runtime.GOARCH)
	}

	if options.Mounter == MounterCosfs && (options.CoreSite != "" || options.GoosefsLite != "") {
		glog.Warningf("cos mounter is %s, core_site and goosefs_lite are invalid", options.Mounter)
	}

	if options.Mounter == MounterGoosefsLite && (options.DebugLevel != "" || options.AdditionalArgs != "") {
		glog.Warningf("cos mounter is %s, dbglevel and additional_args is invalid", options.Mounter)
	}

	return nil
}

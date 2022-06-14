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

package main

import (
	"flag"

	"github.com/golang/glog"
	cos "github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/cosfs"
)

var (
	endpoint   = flag.String("endpoint", "unix://csi/csi.sock", "CSI endpoint")
	driverName = flag.String("drivername", "com.tencent.cloud.csi.cosfs", "name of the driver")
	nodeID     = flag.String("nodeid", "", "node id")
)

func main() {
	flag.Parse()

	if *nodeID == "" {
		glog.Fatal("nodeID is empty")
	}

	driver := cos.NewDriver(*endpoint, *driverName, *nodeID)
	driver.Start()
}

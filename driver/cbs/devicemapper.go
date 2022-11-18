package cbs

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/glog"
)

func mdadmCreate(diskIds string, req *csi.NodeStageVolumeRequest) (string, error) {
	var level = -1
	var devices = -1
	var sLevel, sDevices string
	for k, v := range req.VolumeContext {
		switch strings.ToLower(k) {
		case "level":
			sLevel = v
			var err error
			levelInt, err := strconv.Atoi(v)
			if err != nil {
				glog.Warningf("volumeChargePrepaidPeriod atoi error: %v", err)
			} else {
				level = levelInt
			}
		case "devices":
			sDevices = v
			var err error
			devicesInt, err := strconv.Atoi(v)
			if err != nil {
				glog.Warningf("volumeChargePrepaidPeriod atoi error: %v", err)
			} else {
				devices = devicesInt
			}
		default:
		}
	}
	if level == -1 || devices == -1 {
		return "", fmt.Errorf("level %d or devices %d is invalid", level, devices)
	}

	diskSourceList, err := getDiskSourceListFromDiskIds(diskIds)
	if err != nil {
		return "", fmt.Errorf("getDiskSourceListFromDiskIds %s failed, err: %v", diskIds, err)
	}
	if len(diskSourceList) != devices {
		return "", fmt.Errorf("diskSourceList %d no equal to devices %d", len(diskSourceList), devices)
	}

	mdadmVolArgs := ""
	for _, diskSource := range diskSourceList {
		mdadmVolArgs += diskSource + " "
	}

	pvName := getPVNameFromStagingPath(req.StagingTargetPath)
	if pvName == "" {
		return "", fmt.Errorf("failed to get pv name from staging path: %s", req.StagingTargetPath)
	}
	mdPath := "/dev/md/" + pvName
	mdadmArgs := []string{"--create"}
	mdadmArgs = append(mdadmArgs, mdPath, "--level="+sLevel, "--raid-devices="+sDevices)
	mdadmArgs = append(mdadmArgs, diskSourceList...)
	res, err := exec.Command("mdadm", mdadmArgs...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to mdadm with args: mdadmArgs: %s, mdadmVolArgs: %s, err: %v", mdadmArgs, mdadmVolArgs, err)
	}
	glog.Infof("mdPath is %s, result of mdadm is %s", mdPath, string(res))
	return mdPath, nil
}

func mdadmDelete(diskIds, stagingTargetPath string) error {
	diskSourceList, err := getDiskSourceListFromDiskIds(diskIds)
	if err != nil {
		return fmt.Errorf("getDiskSourceListFromDiskIds %s failed, err: %v", diskIds, err)
	}

	pvName := getPVNameFromStagingPath(stagingTargetPath)
	if pvName == "" {
		return fmt.Errorf("failed to get pv name from staging path: %s", stagingTargetPath)
	}

	mdPath := "/dev/md/" + pvName
	mdadmArgs := []string{"-S", mdPath}
	res, err := exec.Command("mdadm", mdadmArgs...).CombinedOutput()
	if err != nil {
		glog.Errorf("failed to stop mdadm device %s, err: %v", mdPath, err)
		return err
	}
	glog.Infof("success to stop mdadm device %s, response: %s", mdPath, string(res))
	for _, diskSource := range diskSourceList {
		mdadmArgs := []string{"--zero-superblock", diskSource}
		res, err := exec.Command("mdadm", mdadmArgs...).CombinedOutput()
		if err != nil {
			glog.Errorf("failed to clean superblock for device %s, err: %v", diskSource, err)
			return err
		}
		glog.Infof("success to clean superblock for device %s, response: %s", diskSource, string(res))
	}
	return nil
}

func lvmCreate(diskIds string, req *csi.NodeStageVolumeRequest) (string, error) {
	pvName := getPVNameFromStagingPath(req.StagingTargetPath)
	if pvName == "" {
		return "", fmt.Errorf("failed to get pv name from staging path: %s", req.StagingTargetPath)
	}
	pvName = strings.ReplaceAll(pvName, "-", "_")

	var devices = 2
	for k, v := range req.VolumeContext {
		if strings.ToLower(k) == "devices" {
			devicesInt, err := strconv.Atoi(v)
			if err != nil {
				return "", fmt.Errorf("devices atoi failed, err: %v", err)
			} else {
				devices = devicesInt
			}
		}
	}

	diskSourceList, err := getDiskSourceListFromDiskIds(diskIds)
	if err != nil {
		return "", fmt.Errorf("getDiskSourceListFromDiskIds %s failed, err: %v", diskIds, err)
	}
	if len(diskSourceList) != devices {
		return "", fmt.Errorf("diskSourceList %d no equal to devices %d", len(diskSourceList), devices)
	}

	vgName := pvName + "_vg"
	lvName := pvName + "_lv"
	devicePath := "/dev/mapper/" + vgName + "-" + lvName
	noNeedCreate, err := checkLvm(vgName, lvName)
	if err != nil {
		return "", fmt.Errorf("checkLvm failed, err: %v", err)
	}
	if noNeedCreate {
		glog.Infof("lvm for volume %s is already created", diskIds)
		err = lvmVgchange(req.StagingTargetPath, true)
		if err != nil {
			return "", fmt.Errorf("lvmVgchange failed, err: %v", err)
		}
		return devicePath, nil
	}

	lvmVolArgs := ""
	for _, diskSource := range diskSourceList {
		lvmVolArgs += diskSource + " "
		pvCreateArgs := []string{diskSource, "-v"}
		res, err := exec.Command("pvcreate", pvCreateArgs...).CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("failed to pvcreate for diskSource: %s, err: %v", diskSource, err)
		}
		glog.Infof("Result of pvcreate for diskSource %s:\n%s", diskSource, string(res))
	}

	vgCreateArgs := []string{vgName}
	vgCreateArgs = append(vgCreateArgs, diskSourceList...)
	vgCreateArgs = append(vgCreateArgs, "-v")
	res, err := exec.Command("vgcreate", vgCreateArgs...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to vgcreate with args: %s, needCreateDiskSourceList: %v, err: %v", lvmVolArgs, diskSourceList, err)
	}
	glog.Infof("Result of vgcreate for lvmVolArgs %s:\n%s", lvmVolArgs, string(res))

	lvCreateArgs := []string{"-l", "100%FREE", "-n", lvName, "-i", req.VolumeContext["devices"], vgName}
	lvc, err := exec.Command("lvcreate", lvCreateArgs...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to lvcreate %s from vg %s , err: %v", lvName, vgName, err)
	}
	glog.Infof("Result of lvcreate for lvCreateArgs %s:\n%s", lvCreateArgs, string(lvc))

	return devicePath, nil
}

func lvmVgchange(stagingTargetPath string, active bool) error {
	pvName := getPVNameFromStagingPath(stagingTargetPath)
	if pvName == "" {
		return fmt.Errorf("failed to get pv name from staging path: %s", stagingTargetPath)
	}
	pvName = strings.ReplaceAll(pvName, "-", "_")

	vgName := pvName + "_vg"
	vgchangeArgs := make([]string, 2)
	if active {
		vgchangeArgs = []string{"-ay", vgName}
	} else {
		vgchangeArgs = []string{"-an", vgName}
	}

	resVgchange, err := exec.Command("vgchange", vgchangeArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to vgchange for vg %s, err: %v", vgName, err)
	}
	glog.Infof("success to vgchange for vg %s, response:\n%s", vgName, string(resVgchange))

	return nil
}

/*
func lvmDelete(stagingTargetPath string) error {
	pvName := getPVNameFromStagingPath(stagingTargetPath)
	if pvName == "" {
		return fmt.Errorf("failed to get pv name from staging path: %s", stagingTargetPath)
	}
	pvName = strings.ReplaceAll(pvName, "-", "_")

	vgName := pvName + "_vg"
	lvName := pvName + "_lv"

	devicePath := "/dev/" + vgName + "/" + lvName
	lvDeleteArgs := []string{devicePath, "-y"}
	resLvDelete, err := exec.Command("lvremove", lvDeleteArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to lvremove for device %s, err: %v", devicePath, err)
	}
	glog.Infof("success to lvremove for device %s, response:\n%s", devicePath, string(resLvDelete))

	resVgDelete, err := exec.Command("vgremove", vgName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to vgremove for vg %s, err: %v", vgName, err)
	}
	glog.Infof("success to vgremove for vg %s, response:\n%s", vgName, string(resVgDelete))

	return nil
}
*/

func lvmExpand(diskIds, devicePath string, requiredBytes int64) error {
	diskSourceList, err := getDiskSourceListFromDiskIds(diskIds)
	if err != nil {
		return fmt.Errorf("getDiskSourceListFromDiskIds %s failed, err: %v", diskIds, err)
	}
	for _, diskSource := range diskSourceList {
		err = checkVolumePathCapacity(diskSource, requiredBytes)
		if err != nil {
			return fmt.Errorf("check volumePath(%s) capacity failed, error: %v", diskSource, err)
		}
	}

	for _, diskSource := range diskSourceList {
		res, err := exec.Command("pvresize", diskSource).CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to pvresize for diskSource: %s, err: %v", diskSource, err)
		}
		glog.Infof("Result of pvresize for diskSource %s:\n%s", diskSource, string(res))
	}

	lvExtendArgs := []string{devicePath, "-l+100%FREE"}
	resLvExtend, err := exec.Command("lvextend", lvExtendArgs...).CombinedOutput()
	if err != nil && !strings.Contains(err.Error(), "matches existing size") {
		return fmt.Errorf("failed to lvextend for device %s, err: %v", devicePath, err)
	}
	glog.Infof("success to lvextend for device %s, response:\n%s", devicePath, string(resLvExtend))
	return nil
}

func getPVNameFromStagingPath(stagingPath string) string {
	for _, val := range strings.Split(stagingPath, "/") {
		if strings.HasPrefix(val, "pvc-") {
			return val
		}
	}
	return ""
}

func getDiskSourceListFromDiskIds(diskIds string) ([]string, error) {
	diskSourceList := make([]string, 0)
	for _, disk := range strings.Split(diskIds, ",") {
		if strings.HasPrefix(disk, "disk-") {
			diskSource, err := findCBSVolume(disk)
			if err != nil {
				return nil, fmt.Errorf("findCBSVolume failed, cbs disk: %v, err: %v", filepath.Join(DiskByIDDevicePath, DiskByIDDeviceNamePrefix+disk), err)
			}
			diskSourceList = append(diskSourceList, diskSource)
			glog.Infof("the diskSource of %s is %s", disk, diskSource)
		}
	}
	return diskSourceList, nil
}

func checkLvm(vgName, lvName string) (bool, error) {
	res, err := exec.Command("lvs").CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("lvs failed, err: %v", err)
	}
	glog.Infof("Result of lvs:\n%s", string(res))
	return strings.Contains(string(res), vgName) && strings.Contains(string(res), lvName), nil
}

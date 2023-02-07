package chdfs

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang/glog"
	"k8s.io/utils/exec"
	"k8s.io/utils/mount"
)

func createMountPoint(volID, targetPath string) (bool, error) {
	glog.Infof("Creating staging mount point at %s for volume %s", targetPath, volID)

	if !isFileExisted(targetPath) {
		if err := os.MkdirAll(targetPath, 0600); err != nil {
			glog.Errorf("failed to create staging mount point at %s: %v", targetPath, err)
			return false, err
		}
	}

	isMounted, err := isMountPoint(targetPath)
	if err != nil {
		glog.Error("Error in checkout is mount point: ", err)
		return false, err
	}

	return isMounted, nil
}

func isFileExisted(filename string) bool {
	_, err := os.Stat(filename)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return true
}

func isMountPoint(path string) (bool, error) {
	mounter := mount.New("")
	notMnt, err := mounter.IsLikelyNotMountPoint(path)
	if err != nil {
		return false, err
	}
	return !notMnt, nil
}

func Mount(options *chdfsOptions, mountPoint string) error {
	// Construct args
	var args []string
	if options.IsDebug {
		args = append(args, "-debug")
	}
	if options.IsSync {
		args = append(args, "-o", "sync")
	}
	if options.AllowOther {
		args = append(args, "-allow_other")
	}
	args = append(args, mountPoint)

	config, err := prepareConfig(options.Url, options.AdditionalArgs)
	if err != nil {
		return fmt.Errorf("prepareConfig failed, %s", err.Error())
	}
	args = append(args, "--config="+config)

	cmd := "chdfs-fuse"
	err = exec.New().Command(cmd, args...).Start()
	if err != nil {
		return fmt.Errorf("command %s %s failed, err: %v", cmd, strings.Join(args, " "), err)
	}

	time.Sleep(1 * time.Second)
	output, err := exec.New().Command("echo", "$?").CombinedOutput()
	if err != nil {
		return fmt.Errorf("command echo $? failed, output: %s, err: %v", output, err)
	}
	if string(output) != "0" {
		return fmt.Errorf("command %s %s failed", cmd, strings.Join(args, " "))
	}

	return nil
}

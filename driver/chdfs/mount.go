package chdfs

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang/glog"
	"golang.org/x/net/context"
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
	cmd := fmt.Sprintf("chdfs-fuse %s", strings.Join(args, " "))

	glog.Infof("will be exec cmd: %s, start a new goroutine", cmd)
	//start to exec cmd
	go execCmd(cmd)
	// will isMountPoint in ticker
	ticker := time.NewTicker(time.Second * 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*40)
	defer cancel()
	for {
		select {
		case <-ticker.C:
			isMnt, err := isMountPoint(mountPoint)
			if err != nil {
				glog.Errorf("isMountPoint err: %s for mountPoint: %s", err.Error(), mountPoint)
				return fmt.Errorf("isMountPoint err: %s for mountPoint: %s", err.Error(), mountPoint)
			}
			if isMnt {
				glog.Infof("mountPoint %s is already mounted", mountPoint)
				return nil
			}
		case <-ctx.Done():
			glog.Errorf("isMountPoint: %s failed before deadline exceeded", mountPoint)
			return fmt.Errorf("isMountPoint: %s failed before deadline exceeded", mountPoint)
		}
	}
}

func execCmd(cmd string) {
	output, err := exec.New().Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		glog.Errorf("output: %s, error: %v", string(output), err)
	}
	glog.Infof("execCmd end !")
}

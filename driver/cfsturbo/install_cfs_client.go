package cfsturbo

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"os/exec"

	"github.com/golang/glog"
)

const (
	cfsTurboUrl = "https://cfsturbo-client-1251013638.cos.ap-guangzhou.myqcloud.com/tools/cfs_turbo_client_setup"
)

type installer struct {
}

func newInstaller() *installer {
	glog.Infof("Installer is initialization...")
	return &installer{}
}

// Install cfs turbo client
func (in *installer) Install() error {
	//check cfs lustre core kmod install
	err := exec.Command("/bin/bash", "-c", fmt.Sprintf("lsmod | grep %s", CFSTurboLustreKernelModule)).Run()
	if err == nil {
		//已经安装了内核模块
		glog.Infof("node has alreay install kernel mod in node before mount cfs turbo lustre, skip install")
		return nil
	}
	//没有的话直接安装
	glog.Infof("node has not install kernel mod, start install")

	// hold真实的文件系统 root
	oldRootF, err := os.Open("/")
	defer oldRootF.Close()
	if err != nil {
		return fmt.Errorf("hold root system err, message: %s", err.Error())
	}

	// exec chroot
	err = syscall.Chroot("/host")
	defer holdBack(oldRootF)
	if err != nil {
		return fmt.Errorf("chroot /host exec err, message: %s", err.Error())
	}

	// rm -fr
	err = exec.Command("/bin/bash", "-c", "rm -fr /tmp/cfs-turbo*; rm -fr /tmp/cfs_turbo*; rm -fr turbo*").Run()
	if err != nil {
		// rm -fr fail
		return fmt.Errorf("rm -fr /tmp/cfs-turbo err, message: %s", err.Error())
	}
	//wget
	err = exec.Command("/bin/bash", "-c", fmt.Sprintf("wget -O %s %s", "/tmp/cfs_turbo_client_setup", cfsTurboUrl)).Run()
	if err != nil {
		// wget fail
		return fmt.Errorf("wget cfs_turbo_client_setup err, message: %s", err.Error())
	}
	//chmod
	err = exec.Command("/bin/bash", "-c", "chmod +x /tmp/cfs_turbo_client_setup").Run()
	if err != nil {
		// chmod fail
		return fmt.Errorf("chmod +x for cfs_turbo_client_setup err, message: %s", err.Error())
	}
	//exec
	err = exec.Command("/bin/bash", "-c", "/tmp/cfs_turbo_client_setup").Run()
	if err != nil {
		// exec fail
		return fmt.Errorf("exec cfs_turbo_client_setup err, message: %s", err.Error())
	}

	glog.Infof(" install cfs client success !")
	return nil
}

func holdBack(oldRootF *os.File) {
	// switch back
	err := oldRootF.Chdir()
	if err != nil {
		glog.Warningf("chdir() err: %v", err)
	}
	err = syscall.Chroot(".")
	if err != nil {
		glog.Warningf("chroot back err: %v", err)
	}
}

func (in *installer) loop() error {
	for {
		err := in.Install()
		if err == nil {
			glog.Infof("install cfs client success, return")
			return nil
		}
		// need loop again
		glog.Errorf("loop install cfs client failed, message: %s", err.Error())
		time.Sleep(5 * time.Minute)
	}
}

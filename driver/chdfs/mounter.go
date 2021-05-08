package chdfs

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
	Mount(options *chdfsOptions, mountPoint string, config string, configPath string) error
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

func (*defaultMounter) Mount(options *chdfsOptions, mountPoint string, config string, configPath string) error {
	var args []string
	isSync := options.IsSync
	allowOthers := options.AllowOther
	isDebug := options.IsDebug

	// Construct args
	if isDebug {
		args = append(args, "-debug")
	}
	if isSync {
		args = append(args, "-o", "sync")
	}
	if allowOthers {
		args = append(args, "-allow_other")
	}
	args = append(args, mountPoint)
	args = append(args, "--config="+configPath)
	//args = append(args, ">/dev/null 2>&1")
	args = append(args, "&")

	cmd := fmt.Sprintf("nohup chdfs-fuse %s", strings.Join(args, " "))
	err := sendCmdToLauncher(cmd, config, configPath)
	if err != nil {
		glog.Error("Error in sendCmdToLauncher: ", err)
		return err
	}

	glog.Infof("Success mount chdfs.")

	return nil
}

func sendCmdToLauncher(cmd string, config string, configPath string) error {
	httpClient := http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", SocketPath)
			},
		},
	}

	// Send request to unix domain socket
	body := make(map[string]string)
	body["command"] = cmd
	body["config"] = config
	body["configPath"] = configPath
	bodyJson, err := json.Marshal(body)
	if err != nil {
		glog.Error("Error in marshal body: ", err)
		return err
	}

	response, err := httpClient.Post("http://unix/chdfs/launcher", "application/json", strings.NewReader(string(bodyJson)))
	if err != nil {
		glog.Error("Error in send http req to launcher: ", err)
		return err
	}

	defer response.Body.Close()

	respBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("the response of launcher(action: chdfs) is: %v", string(respBody))
	}

	glog.Info("send chdfs command to launcher successfully")
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
	args := []string{
		point,
	}

	cmd := fmt.Sprintf("umount %s", strings.Join(args, " "))
	if err := execCmd(cmd); err != nil {
		glog.Error("Error in exec unmount commond: ", err)
		return err
	}

	return nil
}

func (*defaultMounter) RemoveMountPoint(point string) error {
	return os.Remove(point)
}

func execCmd(cmd string, args ...string) error {
	output, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		return fmt.Errorf("command %s failed: output %s, error: %v", cmd, string(output), err)
	}
	glog.V(4).Infof("command %s succeed: %s", cmd, string(output))
	return nil
}

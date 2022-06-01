package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"k8s.io/utils/exec"
	"k8s.io/utils/mount"
)

const (
	perm = 0600

	SocketPath = "/tmp/cosfs.sock"
)

func main() {
	flag.Parse()

	prepareSocketDir()

	r := mux.NewRouter()
	launcher := r.Path("/launcher").Subrouter()
	launcher.Methods("POST").HandlerFunc(launcherHandler)

	mount := r.Path("/mount").Subrouter()
	mount.Methods("POST").HandlerFunc(mountHandler)

	server := http.Server{
		Handler: r,
	}

	unixListener, err := net.Listen("unix", SocketPath)
	if err != nil {
		glog.Error(err)
		return
	}

	if err := server.Serve(unixListener); err != nil {
		glog.Errorf("cosfs launcher server closed unexpected. %v", err)
	}

	glog.Infoln("launcher server is running.")
	return
}

func prepareSocketDir() {
	if !isFileExisted(SocketPath) {
		pathDir := filepath.Dir(SocketPath)
		if !isFileExisted(pathDir) {
			os.MkdirAll(pathDir, os.ModePerm)
		}
	} else {
		os.Remove(SocketPath)
	}

	glog.Infof("socket dir %s is ready\n", filepath.Dir(SocketPath))
}

func launcherHandler(w http.ResponseWriter, r *http.Request) {
	glog.Infoln("enter launcherHandler...")

	extraFields := make(map[string]string)

	body, err := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		extraFields["errmsg"] = "read request body failed"
		glog.Errorf("%s: %v", extraFields["errmsg"], err)
		generateHttpResponse(w, "failure", http.StatusInternalServerError, extraFields)
		return
	}

	var bodyMap map[string]string
	if err := json.Unmarshal(body, &bodyMap); err != nil {
		extraFields["errmsg"] = "unmarshal request body failed"
		glog.Errorf("%s: %v\n", extraFields["errmsg"], err)
		generateHttpResponse(w, "failure", http.StatusInternalServerError, extraFields)
		return
	}

	cmd, ok := bodyMap["command"]
	if !ok {
		extraFields["errmsg"] = "request body is empty. we need field `command`"
		glog.Errorln(extraFields["errmsg"])
		generateHttpResponse(w, "failure", http.StatusBadRequest, extraFields)
		return
	}

	items := strings.Split(cmd, " ")
	if items[0] == "umount" {
		mounter := mount.New("")
		notMnt, err := mounter.IsLikelyNotMountPoint(items[len(items)-1])
		if err != nil {
			if strings.Contains(err.Error(), "endpoint is not connected") {
				notMnt = false
			} else {
				extraFields["errmsg"] = fmt.Sprintf("check IsLikelyNotMountPoint failed. %v", err)
				glog.Errorln(extraFields["errmsg"])
				generateHttpResponse(w, "failure", http.StatusInternalServerError, extraFields)
				return
			}
		}

		if notMnt {
			glog.Infof("%s not mounted, assume umount success.", items[len(items)-1])
			generateHttpResponse(w, "success", http.StatusOK, extraFields)
			return
		}
	}

	if err := execCmd(cmd); err != nil {
		extraFields["errmsg"] = fmt.Sprintf("exec command(%s) failed. %v", cmd, err)
		glog.Errorln(extraFields["errmsg"])
		generateHttpResponse(w, "failure", http.StatusInternalServerError, extraFields)
		return
	}
	glog.Infof("exec command %s success.\n", cmd)

	generateHttpResponse(w, "success", http.StatusOK, extraFields)
}

func mountHandler(w http.ResponseWriter, r *http.Request) {
	glog.Infoln("enter mountHandler...")
	extraFields := make(map[string]string)

	body, err := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		extraFields["errmsg"] = "read request body failed"
		glog.Errorf("%s: %v", extraFields["errmsg"], err)
		generateHttpResponse(w, "fail", http.StatusInternalServerError, extraFields)
		return
	}

	var bodyMap map[string]string
	if err := json.Unmarshal(body, &bodyMap); err != nil {
		extraFields["errmsg"] = "unmarshal request body failed"
		glog.Errorf("%s: %v", extraFields["errmsg"], err)
		generateHttpResponse(w, "fail", http.StatusInternalServerError, extraFields)
		return
	}

	stagingPath, ok := bodyMap["stagingPath"]
	if !ok {
		extraFields["errmsg"] = "request body is empty. we need field `staingPath`"
		glog.Errorln(extraFields["errmsg"])
		generateHttpResponse(w, "fail", http.StatusBadRequest, extraFields)
		return
	}

	if !isFileExisted(stagingPath) {
		glog.Infof("staging mount point path %s is not exist.\n", stagingPath)
		if err := createMountPoint(stagingPath); err != nil {
			glog.Errorf("failed to create staging mount point at %s: %v", stagingPath, err)
			extraFields["errmsg"] = fmt.Sprintf("create staging mount point failed: %s", err)
			generateHttpResponse(w, "fail", http.StatusInternalServerError, extraFields)
			return
		}
	}

	isMounted, err := isMountPoint(stagingPath)
	if err != nil {
		extraFields["errmsg"] = fmt.Sprintf("failed to check whether staging point mounted or not: %v", err)
		glog.Errorln(extraFields["errmsg"])
		generateHttpResponse(w, "fail", http.StatusInternalServerError, extraFields)
		return
	}

	if isMounted {
		glog.Infof("the staging path(%s) is a mount point\n", stagingPath)
	} else {
		glog.Infof("the staging path(%s) is not a mount point\n", stagingPath)
	}

	extraFields["isMounted"] = strconv.FormatBool(isMounted)
	generateHttpResponse(w, "success", http.StatusOK, extraFields)
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

func execCmd(cmd string) error {
	e := exec.New()
	output, err := e.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		return fmt.Errorf("command %s failed: output %s, error: %v", cmd, string(output), err)
	}
	glog.V(4).Infof("command %s succeed: %s", cmd, string(output))
	return nil
}

func generateHttpResponse(w http.ResponseWriter, result string, statusCode int, extra map[string]string) {
	res := make(map[string]string)
	res["result"] = result
	for k, v := range extra {
		res[k] = v
	}

	response, _ := json.Marshal(res)
	w.WriteHeader(statusCode)
	w.Header().Set("Content-Type", "application/json")
	w.Write(response)
}

func createMountPoint(point string) error {
	return os.MkdirAll(point, perm)
}

func isMountPoint(path string) (bool, error) {
	mounter := mount.New("")
	notMnt, err := mounter.IsLikelyNotMountPoint(path)
	if err != nil {
		return false, err
	}
	return !notMnt, nil
}

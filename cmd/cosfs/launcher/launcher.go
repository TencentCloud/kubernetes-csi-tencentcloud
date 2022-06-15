package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
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

	SocketPath         = "/etc/csi-cos/cosfs.sock"
	MounterCosfs       = "cosfs"
	MounterGoosefsLite = "/goosefs-lite/bin/goosefs-lite"
	CosfsPasswdPrefix  = "-opasswd_file="
	RequestCommand     = "command"
	RequestTargetPath  = "targetPath"
	ResponseIsMounted  = "isMounted"
	ResponseSuccess    = "success"
	ResponseFailure    = "failure"
	ResponseErrmsg     = "errmsg"
	ResponseResult     = "result"
)

func main() {
	flag.Parse()

	if isFileExisted(SocketPath) {
		if err := os.Remove(SocketPath); err != nil {
			glog.Fatalf("remove %s failed, error: %v", SocketPath, err)
		}
	}

	r := mux.NewRouter()
	create := r.Path("/create").Subrouter()
	create.Methods("POST").HandlerFunc(createHandler)

	mount := r.Path("/mount").Subrouter()
	mount.Methods("POST").HandlerFunc(mountHandler)

	umount := r.Path("/umount").Subrouter()
	umount.Methods("POST").HandlerFunc(umountHandler)

	server := http.Server{
		Handler: r,
	}

	unixListener, err := net.Listen("unix", SocketPath)
	if err != nil {
		glog.Fatalf("net.Listen failed, error: %v", err)
	}

	glog.Info("cos launcher starting")
	if err := server.Serve(unixListener); err != nil {
		glog.Fatalf("cos launcher server closed unexpected, error: %v", err)
	}
}

func createHandler(w http.ResponseWriter, r *http.Request) {
	glog.Infoln("enter createHandler...")
	extraFields := make(map[string]string)

	bodyMap := getRequestBodyMap(extraFields, w, r)
	if bodyMap == nil {
		return
	}

	targetPath, ok := bodyMap[RequestTargetPath]
	if !ok {
		extraFields[ResponseErrmsg] = "no field `targetPath` in request body"
		glog.Errorf("error: %s", extraFields[ResponseErrmsg])
		generateHttpResponse(w, ResponseFailure, http.StatusBadRequest, extraFields)
		return
	}

	if !isFileExisted(targetPath) {
		glog.Infof("target path %s is not exist.", targetPath)
		if err := os.MkdirAll(targetPath, perm); err != nil {
			extraFields[ResponseErrmsg] = "create target path failed"
			glog.Errorf("%s, error: %v", extraFields[ResponseErrmsg], err)
			generateHttpResponse(w, ResponseFailure, http.StatusInternalServerError, extraFields)
			return
		}
	}

	isMounted, err := isMountPoint(targetPath)
	if err != nil {
		extraFields[ResponseErrmsg] = "check target path mounted or not failed"
		glog.Errorf("%s, error: %v", extraFields[ResponseErrmsg], err)
		generateHttpResponse(w, ResponseFailure, http.StatusInternalServerError, extraFields)
		return
	}

	if isMounted {
		glog.Infof("the target path(%s) is a mount point", targetPath)
	} else {
		glog.Infof("the target path(%s) is not a mount point", targetPath)
	}

	extraFields[ResponseIsMounted] = strconv.FormatBool(isMounted)
	generateHttpResponse(w, ResponseSuccess, http.StatusOK, extraFields)
}

func mountHandler(w http.ResponseWriter, r *http.Request) {
	glog.Infoln("enter mountHandler...")
	extraFields := make(map[string]string)

	bodyMap := getRequestBodyMap(extraFields, w, r)
	if bodyMap == nil {
		return
	}

	cmd, ok := bodyMap[RequestCommand]
	if !ok {
		extraFields[ResponseErrmsg] = "no field `command` in request body"
		glog.Errorf("error: %s", extraFields[ResponseErrmsg])
		generateHttpResponse(w, ResponseFailure, http.StatusBadRequest, extraFields)
		return
	}

	if err := execCmd(cmd); err != nil {
		extraFields[ResponseErrmsg] = fmt.Sprintf("exec command %s failed, %v", cmd, err)
		glog.Errorln(extraFields[ResponseErrmsg])
		generateHttpResponse(w, ResponseFailure, http.StatusInternalServerError, extraFields)
		return
	}

	cmds := strings.Split(cmd, " ")
	switch cmds[0] {
	case MounterCosfs:
		for _, v := range cmds {
			if strings.Contains(v, CosfsPasswdPrefix) {
				if err := os.RemoveAll(filepath.Dir(strings.TrimPrefix(v, CosfsPasswdPrefix))); err != nil {
					glog.Warning("remove cosfs configs failed: %v", err)
				}
			}
		}
	case MounterGoosefsLite:
		for k, v := range cmds {
			if v == "-c" {
				if err := os.RemoveAll(filepath.Dir(cmds[k+1])); err != nil {
					glog.Warning("remove goosefs-lite configs failed: %v", err)
				}
			}
		}
	}

	glog.Infof("exec command %s success", cmd)
	generateHttpResponse(w, ResponseSuccess, http.StatusOK, extraFields)
}

func umountHandler(w http.ResponseWriter, r *http.Request) {
	glog.Infoln("enter umountHandler...")
	extraFields := make(map[string]string)

	bodyMap := getRequestBodyMap(extraFields, w, r)
	if bodyMap == nil {
		return
	}

	cmd, ok := bodyMap[RequestCommand]
	if !ok {
		extraFields[ResponseErrmsg] = "no field `command` in request body"
		glog.Errorf("error: %s", extraFields[ResponseErrmsg])
		generateHttpResponse(w, ResponseFailure, http.StatusBadRequest, extraFields)
		return
	}

	items := strings.Split(cmd, " ")
	isMounted, err := isMountPoint(items[len(items)-1])
	if err != nil {
		if strings.Contains(err.Error(), "endpoint is not connected") {
			isMounted = true
		} else {
			extraFields[ResponseErrmsg] = "check target path mounted or not failed"
			glog.Errorf("%s, error: %v", extraFields[ResponseErrmsg], err)
			generateHttpResponse(w, ResponseFailure, http.StatusInternalServerError, extraFields)
			return
		}
	}

	if !isMounted {
		glog.Infof("%s not mounted, assume umount success.", items[len(items)-1])
		generateHttpResponse(w, ResponseSuccess, http.StatusOK, extraFields)
		return
	}

	if err := execCmd(cmd); err != nil {
		extraFields[ResponseErrmsg] = fmt.Sprintf("exec command failed, %v", err)
		glog.Errorln(extraFields[ResponseErrmsg])
		generateHttpResponse(w, ResponseFailure, http.StatusInternalServerError, extraFields)
		return
	}
	glog.Infof("exec command %s success.\n", cmd)

	generateHttpResponse(w, ResponseSuccess, http.StatusOK, extraFields)
}

func getRequestBodyMap(extraFields map[string]string, w http.ResponseWriter, r *http.Request) map[string]string {
	body, err := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		extraFields[ResponseErrmsg] = "read request body failed"
		glog.Errorf("%s, error: %v", extraFields[ResponseErrmsg], err)
		generateHttpResponse(w, ResponseFailure, http.StatusInternalServerError, extraFields)
		return nil
	}

	var bodyMap map[string]string
	if err := json.Unmarshal(body, &bodyMap); err != nil {
		extraFields[ResponseErrmsg] = "unmarshal request body failed"
		glog.Errorf("%s, error: %v", extraFields[ResponseErrmsg], err)
		generateHttpResponse(w, ResponseFailure, http.StatusInternalServerError, extraFields)
		return nil
	}

	return bodyMap
}

func generateHttpResponse(w http.ResponseWriter, result string, statusCode int, extra map[string]string) {
	res := make(map[string]string)
	res[ResponseResult] = result
	for k, v := range extra {
		res[k] = v
	}

	response, _ := json.Marshal(res)
	w.WriteHeader(statusCode)
	w.Header().Set("Content-Type", "application/json")
	w.Write(response)
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

func execCmd(cmd string) error {
	e := exec.New()
	output, err := e.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		return fmt.Errorf("output: %s, error: %v", string(output), err)
	}
	return nil
}

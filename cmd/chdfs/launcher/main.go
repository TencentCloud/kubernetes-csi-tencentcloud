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

	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"k8s.io/utils/exec"
)

const (
	perm = 0600

	SocketPath = "/tmp/chdfs.sock"
)

func main() {
	flag.Parse()

	err := prepareSocketDir()
	if err != nil {
		glog.Errorf("create SocketPath failed, error: %v", err)
	}

	r := mux.NewRouter()
	launcher := r.Path("/chdfs/launcher").Subrouter()
	launcher.Methods("POST").HandlerFunc(launcherHandler)

	server := http.Server{
		Handler: r,
	}

	unixListener, err := net.Listen("unix", SocketPath)
	if err != nil {
		glog.Error(err)
		return
	}

	if err := server.Serve(unixListener); err != nil {
		glog.Errorf("chdfs launcher server closed unexpected. %v", err)
		os.Exit(1)
	}

	glog.Infoln("launcher server is running.")
	return
}

func prepareSocketDir() error {
	if !isFileExisted(SocketPath) {
		pathDir := filepath.Dir(SocketPath)
		if !isFileExisted(pathDir) {
			if err := os.MkdirAll(pathDir, os.ModePerm); err != nil {
				return err
			}
		}
	} else {
		if err := os.Remove(SocketPath); err != nil {
			return err
		}
	}

	glog.Infof("dir %s is ready\n", filepath.Dir(SocketPath))
	return nil
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
		extraFields["errmsg"] = "request body not contains field `command`"
		glog.Errorln(extraFields["errmsg"])
		generateHttpResponse(w, "failure", http.StatusBadRequest, extraFields)
		return
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

package cfsturbo

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/golang/glog"
)

const (
	cfsturboGlobalPath = "/etc/cfsturbo/global"
	cfsturboConfigName = "cfsturboConfig"
)

type File struct {
	mu   sync.RWMutex
	File string
}

func (f *File) SafeWrite(data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	return ioutil.WriteFile(f.File, data, 0644)
}

func WriteCfsturboConfig(fsConfig map[string][]string) error {
	val, err := json.Marshal(fsConfig)
	if err != nil {
		glog.Errorf("Marshal fsConfig failed, err: %v", err)
		return err
	}

	filepath := path.Join(cfsturboGlobalPath, cfsturboConfigName)
	file := File{
		mu:   sync.RWMutex{},
		File: filepath,
	}
	err = file.SafeWrite(val)
	if err != nil {
		return err
	}

	return nil
}

func LoadCfsturboConfig() (map[string][]string, error) {
	fsConfig := make(map[string][]string)

	_, err := os.Stat(path.Join(cfsturboGlobalPath, cfsturboConfigName))
	if err != nil {
		if os.IsNotExist(err) {
			err := WriteCfsturboConfig(fsConfig)
			if err != nil {
				return nil, err
			}
			return fsConfig, nil
		} else {
			return nil, err
		}
	}

	fileContents, err := ioutil.ReadFile(path.Join(cfsturboGlobalPath, cfsturboConfigName))
	if err != nil {
		glog.Errorf("Read cfsturboConfig %s failed, err: %v", path.Join(cfsturboGlobalPath, cfsturboConfigName), err)
		return nil, err
	}

	err = json.Unmarshal(fileContents, &fsConfig)
	if err != nil {
		glog.Errorf("Unmarshal fsConfig failed, err: %v", err)
		return nil, err
	}

	return fsConfig, nil
}

func UpdateCfsturboConfig(fsid, volumeId string) error {
	needUpdate := false

	cfsturboConfig, err := LoadCfsturboConfig()
	if err != nil {
		return err
	}

	for id, volumeIds := range cfsturboConfig {
		if id == fsid {
			continue
		}
		for n, vid := range volumeIds {
			if vid == volumeId {
				volumeIds = append(volumeIds[:n], volumeIds[n+1:]...)
				cfsturboConfig[id] = volumeIds
				needUpdate = true
			}
		}
	}

	if _, ok := cfsturboConfig[fsid]; !ok {
		cfsturboConfig[fsid] = []string{volumeId}
		needUpdate = true
	}

	if !strings.Contains(strings.Join(cfsturboConfig[fsid], ","), volumeId) {
		cfsturboConfig[fsid] = append(cfsturboConfig[fsid], volumeId)
		needUpdate = true
	}

	if needUpdate {
		glog.Infof("Update cfsturboConfig: %v", cfsturboConfig)
		err := WriteCfsturboConfig(cfsturboConfig)
		if err != nil {
			return err
		}
	}

	return nil
}

func GetFSIDfromCfsturboConfig(volumeId string) (string, error) {
	cfsturboConfig, err := LoadCfsturboConfig()
	if err != nil {
		return "", err
	}

	for fsid, volumeIds := range cfsturboConfig {
		for n, vid := range volumeIds {
			if vid == volumeId {
				if len(volumeIds) == 1 {
					return fsid, nil
				}
				volumeIds = append(volumeIds[:n], volumeIds[n+1:]...)
				cfsturboConfig[fsid] = volumeIds
				err := WriteCfsturboConfig(cfsturboConfig)
				if err != nil {
					return "", err
				}
				return "", nil
			}
		}
	}

	return "", nil
}

func DeleteFSIDInCfsturboConfig(mountPath, fsid string) error {
	needDelete := false

	_, err := os.Stat(mountPath)
	if err != nil {
		if os.IsNotExist(err) {
			needDelete = true
		} else {
			return err
		}
	}

	if needDelete {
		cfsturboConfig, err := LoadCfsturboConfig()
		if err != nil {
			return err
		}

		delete(cfsturboConfig, fsid)
		err = WriteCfsturboConfig(cfsturboConfig)
		if err != nil {
			return err
		}
	}

	return nil
}

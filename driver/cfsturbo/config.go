package cfsturbo

import (
	"encoding/json"
	"io/fs"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	"github.com/golang/glog"
)

const (
	cfsturboGlobalPath = "/etc/cfsturbo/global"
	cfsturboConfigPath = "/etc/cfsturbo/global/config"
)

func WriteCfsturboConfig(cfsturboConfig []string, fsidWithRootDir string) error {
	val, err := json.Marshal(cfsturboConfig)
	if err != nil {
		glog.Errorf("Marshal cfsturboConfig failed, err: %v", err)
		return err
	}

	configPath := path.Join(cfsturboConfigPath, fsidWithRootDir)
	err = ioutil.WriteFile(configPath, val, 0644)
	if err != nil {
		return err
	}

	return nil
}

func GetCfsturboConfigByFSIDWithRootDir(fsidWithRootDir string) ([]string, error) {
	cfsturboConfig := make([]string, 0)

	_, err := os.Stat(path.Join(cfsturboConfigPath, fsidWithRootDir))
	if err != nil {
		if os.IsNotExist(err) {
			glog.Infof("Create empty cfsturboConfig %s", fsidWithRootDir)
			err := WriteCfsturboConfig(cfsturboConfig, fsidWithRootDir)
			if err != nil {
				glog.Errorf("Create empty cfsturboConfig %s failed, err: %v", fsidWithRootDir, err)
				return nil, err
			}
			return cfsturboConfig, nil
		} else {
			return nil, err
		}
	}

	fileContents, err := ioutil.ReadFile(path.Join(cfsturboConfigPath, fsidWithRootDir))
	if err != nil {
		glog.Errorf("Read cfsturboConfig %s failed, err: %v", fsidWithRootDir, err)
		return nil, err
	}

	err = json.Unmarshal(fileContents, &cfsturboConfig)
	if err != nil {
		glog.Errorf("Unmarshal cfsturboConfig %s failed, err: %v", fsidWithRootDir, err)
		return nil, err
	}

	return cfsturboConfig, nil
}

func AddVolumeIdToCfsturboConfig(fsidWithRootDir, volumeId string) error {
	_, err := os.Stat(cfsturboConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			glog.Infof("Create cfsturboConfigPath %s", cfsturboConfigPath)
			err := os.MkdirAll(cfsturboConfigPath, 0644)
			if err != nil {
				glog.Errorf("Create cfsturboConfigPath %s failed, err: %v", cfsturboConfigPath, err)
				return err
			}
		} else {
			return err
		}
	}

	cfsturboConfig, err := GetCfsturboConfigByFSIDWithRootDir(fsidWithRootDir)
	if err != nil {
		return err
	}

	for _, vid := range cfsturboConfig {
		if vid == volumeId {
			return nil
		}
	}

	glog.Infof("Add volumeId %s to cfsturboConfig %s: %v", volumeId, fsidWithRootDir, cfsturboConfig)
	cfsturboConfig = append(cfsturboConfig, volumeId)
	err = WriteCfsturboConfig(cfsturboConfig, fsidWithRootDir)
	if err != nil {
		return err
	}

	return nil
}

func GetFSIDWithRootDirByVolumeId(volumeId string) (string, error) {
	cfsturboConfigs, err := LoadCfsturboConfigs()
	if err != nil {
		return "", err
	}

	for fsidWithRootDir, cfsturboConfig := range cfsturboConfigs {
		for _, vid := range cfsturboConfig {
			if vid == volumeId {
				return fsidWithRootDir, nil
			}
		}
	}

	return "", nil
}

func LoadCfsturboConfigs() (map[string][]string, error) {
	cfsturboConfigs := make(map[string][]string)

	if _, err := os.Stat(cfsturboConfigPath); err != nil {
		if os.IsNotExist(err) {
			return cfsturboConfigs, nil
		}
		return nil, err
	}

	err := filepath.Walk(cfsturboConfigPath, func(path string, info fs.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}

		fileContents, err := ioutil.ReadFile(path)
		if err != nil {
			glog.Errorf("Read cfsturboConfig %s failed, err: %v", info.Name(), err)
			return err
		}

		cfsturboConfig := make([]string, 0)
		err = json.Unmarshal(fileContents, &cfsturboConfig)
		if err != nil {
			glog.Errorf("Unmarshal cfsturboConfig %s failed, err: %v", info.Name(), err)
			return err
		}

		cfsturboConfigs[info.Name()] = cfsturboConfig
		return nil
	})
	if err != nil {
		return nil, err
	}

	return cfsturboConfigs, nil
}

func DeleteVolumeIdFromCfsturboConfig(volumeId, fsidWithRootDir string) (bool, error) {
	cfsturboConfig, err := GetCfsturboConfigByFSIDWithRootDir(fsidWithRootDir)
	if err != nil {
		glog.Errorf("Get cfsturboConfig by FSIDWithRootDir %s failed, err: %v", fsidWithRootDir, err)
		return false, err
	}

	if len(cfsturboConfig) == 1 {
		if cfsturboConfig[0] == volumeId {
			return true, nil
		}
		return false, nil
	}

	glog.Infof("Delete volumeId %s from cfsturboConfig %s: %v", volumeId, fsidWithRootDir, cfsturboConfig)
	for n, vid := range cfsturboConfig {
		if vid == volumeId {
			cfsturboConfig = append(cfsturboConfig[:n], cfsturboConfig[n+1:]...)
			break
		}
	}
	err = WriteCfsturboConfig(cfsturboConfig, fsidWithRootDir)
	if err != nil {
		return false, err
	}

	return false, nil
}

func DeleteCfsturboConfig(fsidWithRootDir string) error {
	glog.Infof("Delete cfsturboConfig %s", fsidWithRootDir)
	return os.RemoveAll(path.Join(cfsturboConfigPath, fsidWithRootDir))
}

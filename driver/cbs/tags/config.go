package tags

import (
	"encoding/json"
	"io/ioutil"
	"os"

	"github.com/golang/glog"
)

const (
	ConfigPath = "/etc/cbs-csi-config"
)

// GetConfigTags 获取 config 中的标签
func GetConfigTags() (map[string]string, error) {
	ConfigTags := make(map[string]string)

	_, err := os.Stat(ConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			glog.Infof("Create empty config %s", ConfigPath)
			err := UpdateConfigTags(ConfigTags)
			if err != nil {
				glog.Errorf("Create empty config %s failed, err: %v", ConfigPath, err)
				return nil, err
			}
			return ConfigTags, nil
		} else {
			return nil, err
		}
	}

	fileContents, err := ioutil.ReadFile(ConfigPath)
	if err != nil {
		glog.Errorf("Read config failed, err: %v", err)
		return nil, err
	}

	err = json.Unmarshal(fileContents, &ConfigTags)
	if err != nil {
		glog.Errorf("Unmarshal config failed, err: %v", err)
		return nil, err
	}

	return ConfigTags, nil
}

// UpdateConfigTags 更新 config
func UpdateConfigTags(ConfigTags map[string]string) error {
	val, err := json.Marshal(ConfigTags)
	if err != nil {
		glog.Errorf("Marshal config failed, err: %v", err)
		return err
	}

	err = ioutil.WriteFile(ConfigPath, val, 0644)
	if err != nil {
		return err
	}

	return nil
}

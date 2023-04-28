package chdfs

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/golang/glog"
)

var chdfsConfigTemplate = template.Must(template.New("config").Parse(`
[proxy]
url="http://{{.Proxy.Url}}"

[security]
ssl-ca-path="{{.Security.SslCaPath}}"

[client]
renew-session-lease-time-sec={{.Client.RenewSessionLeaseTimeSec}}
mount-point="{{.Client.MountPoint}}"
mount-sub-dir="{{.Client.MountSubDir}}"
user="{{.Client.User}}"
group="{{.Client.Group}}"
force-sync={{.Client.ForceSync}}

[cache]
update-sts-time-sec={{.Cache.UpdateStsTimeSec}}
cos-client-timeout-sec={{.Cache.CosClientTimeoutSec}}
inode-attr-expired-time-sec={{.Cache.InodeAttrExpiredTimeSec}}

[cache.read]
block-expired-time-sec={{.Cache.Read.BlockExpiredTimeSec}}
max-block-num={{.Cache.Read.MaxBlockNum}}
read-ahead-block-num={{.Cache.Read.ReadAheadBlockNum}}
max-cos-load-qps={{.Cache.Read.MaxCosLoadQps}}
load-thread-num={{.Cache.Read.LoadThreadNum}}
select-thread-num={{.Cache.Read.SelectThreadNum}}
rand-read={{.Cache.Read.RandRead}}

[cache.write]
max-mem-table-range-num={{.Cache.Write.MaxMemTableRangeNum}}
max-mem-table-size-mb={{.Cache.Write.MaxMemTableSizeMb}}
max-cos-flush-qps={{.Cache.Write.MaxCosFlushQps}}
flush-thread-num={{.Cache.Write.FlushThreadNum}}
commit-queue-len={{.Cache.Write.CommitQueueLen}}
max-commit-heap-size={{.Cache.Write.MaxCommitHeapSize}}
auto-merge={{.Cache.Write.AutoMerge}}
auto-sync={{.Cache.Write.AutoSync}}
auto-sync-time-ms={{.Cache.Write.AutoSyncTimeMs}}

[log]
level="{{.Log.Level}}"

[log.file]
filename="{{.LogFile.Filename}}"
log-rotate={{.LogFile.LogRotate}}
max-size={{.LogFile.MaxSize}}
max-days={{.LogFile.MaxDays}}
max-backups={{.LogFile.MaxBackups}}

`[1:]))

type ChdfsConfig struct {
	Proxy    Proxy
	Security Security
	Client   Client
	Cache    Cache
	Log      Log
	LogFile  LogFile
}

type Proxy struct {
	Url string
}

type Security struct {
	SslCaPath string
}

type Client struct {
	RenewSessionLeaseTimeSec int
	MountPoint               string
	MountSubDir              string
	User                     string
	Group                    string
	ForceSync                bool
}

type Cache struct {
	UpdateStsTimeSec        int
	CosClientTimeoutSec     int
	InodeAttrExpiredTimeSec int
	Read                    Read
	Write                   Write
}

type Read struct {
	BlockExpiredTimeSec int
	MaxBlockNum         int
	ReadAheadBlockNum   int
	MaxCosLoadQps       int
	LoadThreadNum       int
	SelectThreadNum     int
	RandRead            bool
}

type Write struct {
	MaxMemTableRangeNum int
	MaxMemTableSizeMb   int
	MaxCosFlushQps      int
	FlushThreadNum      int
	CommitQueueLen      int
	MaxCommitHeapSize   int
	AutoMerge           bool
	AutoSync            bool
	AutoSyncTimeMs      int
}

type Log struct {
	Level string
}

type LogFile struct {
	Filename   string
	LogRotate  bool
	MaxSize    int
	MaxDays    int
	MaxBackups int
}

type ArgsErr struct {
	Key string
	Err string
}

func NewDefaultChdfsConfig(url, mountPoint string) *ChdfsConfig {
	return &ChdfsConfig{
		Proxy: Proxy{
			Url: url,
		},
		Client: Client{
			RenewSessionLeaseTimeSec: 10,
			MountPoint:               mountPoint,
			MountSubDir:              "/",
			ForceSync:                false,
		},
		Cache: Cache{
			UpdateStsTimeSec:        30,
			CosClientTimeoutSec:     5,
			InodeAttrExpiredTimeSec: 30,
			Read: Read{
				BlockExpiredTimeSec: 10,
				MaxBlockNum:         256,
				ReadAheadBlockNum:   15,
				MaxCosLoadQps:       1024,
				LoadThreadNum:       128,
				SelectThreadNum:     64,
				RandRead:            false,
			},
			Write: Write{
				MaxMemTableRangeNum: 32,
				MaxMemTableSizeMb:   64,
				MaxCosFlushQps:      256,
				FlushThreadNum:      128,
				CommitQueueLen:      100,
				MaxCommitHeapSize:   500,
				AutoMerge:           true,
				AutoSync:            false,
				AutoSyncTimeMs:      1000,
			},
		},
		Log: Log{
			Level: "info",
		},
		LogFile: LogFile{
			Filename:   "./log/chdfs.log",
			LogRotate:  true,
			MaxSize:    2000,
			MaxDays:    7,
			MaxBackups: 100,
		},
	}
}

func prepareConfig(url, additionalArgs string) (string, error) {
	mountPoint := strings.Split(url, ".")[0]
	if mountPoint == "" {
		return "", fmt.Errorf("get MountPoint from Url %s failed", url)
	}

	chdfsConfig, err := NewChdfsConfig(url, mountPoint, additionalArgs)
	if err != nil {
		return "", fmt.Errorf("error create ChdfsConfig: %s", err.Error())
	}

	var buff = bytes.NewBuffer([]byte{})
	err = chdfsConfigTemplate.Execute(buff, chdfsConfig)
	if err != nil {
		return "", fmt.Errorf("error Execute chdfsConfigTemplate: %s", err.Error())
	}
	configName := mountPoint

	// add subDir
	if chdfsConfig.Client.MountSubDir != "" && chdfsConfig.Client.MountSubDir != "/" {
		configName = fmt.Sprintf("%s%s", mountPoint, strings.ReplaceAll(chdfsConfig.Client.MountSubDir, "/", "-"))
	}
	glog.Infof("configName: %s, subDir: %s", configName, chdfsConfig.Client.MountSubDir)
	config := "/etc/chdfs/" + configName + ".conf"
	conf, err := ioutil.ReadAll(buff)
	if err != nil {
		return "", fmt.Errorf("error resolve template: %s", err.Error())
	}

	err = WriteFile(config, string(conf))
	if err != nil {
		return "", fmt.Errorf("error Write config: %s", err.Error())
	}

	return config, nil
}

func NewChdfsConfig(url, mountPoint, additionalArgs string) (*ChdfsConfig, error) {
	chdfsConfig := NewDefaultChdfsConfig(url, mountPoint)
	if additionalArgs == "" {
		return chdfsConfig, nil
	}

	argsMap := make(map[string]string, 0)
	args := strings.Split(additionalArgs, " ")
	for _, arg := range args {
		a := strings.Split(arg, "=")
		if len(a) != 2 {
			return nil, fmt.Errorf("invalid argument in additionalArgs %s", additionalArgs)
		}
		argsMap[a[0]] = a[1]
	}

	var err error
	var errs []ArgsErr
	for k, v := range argsMap {
		switch strings.ToLower(k) {
		case "security.ssl-ca-path":
			chdfsConfig.Security.SslCaPath = v
		case "client.renew-session-lease-time-sec":
			chdfsConfig.Client.RenewSessionLeaseTimeSec, err = Num(v)
			if err != nil {
				errs = append(errs, ArgsErr{k, err.Error()})
			}
		case "client.mount-sub-dir":
			chdfsConfig.Client.MountSubDir = v
		case "client.user":
			chdfsConfig.Client.User = v
		case "client.group":
			chdfsConfig.Client.Group = v
		case "client.force-sync":
			chdfsConfig.Client.ForceSync = isTrue(v)
			if err != nil {
				errs = append(errs, ArgsErr{k, err.Error()})
			}
		case "cache.update-sts-time-sec":
			chdfsConfig.Cache.UpdateStsTimeSec, err = Num(v)
			if err != nil {
				errs = append(errs, ArgsErr{k, err.Error()})
			}
		case "cache.cos-client-timeout-sec":
			chdfsConfig.Cache.CosClientTimeoutSec, err = Num(v)
			if err != nil {
				errs = append(errs, ArgsErr{k, err.Error()})
			}
		case "cache.inode-attr-expired-time-sec":
			chdfsConfig.Cache.InodeAttrExpiredTimeSec, err = Num(v)
			if err != nil {
				errs = append(errs, ArgsErr{k, err.Error()})
			}
		case "cache.read.block-expired-time-sec":
			chdfsConfig.Cache.Read.BlockExpiredTimeSec, err = Num(v)
			if err != nil {
				errs = append(errs, ArgsErr{k, err.Error()})
			}
		case "cache.read.max-block-num":
			chdfsConfig.Cache.Read.MaxBlockNum, err = Num(v)
			if err != nil {
				errs = append(errs, ArgsErr{k, err.Error()})
			}
		case "cache.read.read-ahead-block-num":
			chdfsConfig.Cache.Read.ReadAheadBlockNum, err = Num(v)
			if err != nil {
				errs = append(errs, ArgsErr{k, err.Error()})
			}
		case "cache.read.max-cos-load-qps":
			chdfsConfig.Cache.Read.MaxCosLoadQps, err = Num(v)
			if err != nil {
				errs = append(errs, ArgsErr{k, err.Error()})
			}
		case "cache.read.load-thread-num":
			chdfsConfig.Cache.Read.LoadThreadNum, err = Num(v)
			if err != nil {
				errs = append(errs, ArgsErr{k, err.Error()})
			}
		case "cache.read.select-thread-num":
			chdfsConfig.Cache.Read.SelectThreadNum, err = Num(v)
			if err != nil {
				errs = append(errs, ArgsErr{k, err.Error()})
			}
		case "cache.read.rand-read":
			chdfsConfig.Cache.Read.RandRead, err = IsTrue(v)
			if err != nil {
				errs = append(errs, ArgsErr{k, err.Error()})
			}
		case "cache.write.max-mem-table-range-num":
			chdfsConfig.Cache.Write.MaxMemTableRangeNum, err = Num(v)
			if err != nil {
				errs = append(errs, ArgsErr{k, err.Error()})
			}
		case "cache.write.max-mem-table-size-mb":
			chdfsConfig.Cache.Write.MaxMemTableSizeMb, err = Num(v)
			if err != nil {
				errs = append(errs, ArgsErr{k, err.Error()})
			}
		case "cache.write.max-cos-flush-qps":
			chdfsConfig.Cache.Write.MaxCosFlushQps, err = Num(v)
			if err != nil {
				errs = append(errs, ArgsErr{k, err.Error()})
			}
		case "cache.write.flush-thread-num":
			chdfsConfig.Cache.Write.FlushThreadNum, err = Num(v)
			if err != nil {
				errs = append(errs, ArgsErr{k, err.Error()})
			}
		case "cache.write.commit-queue-len":
			chdfsConfig.Cache.Write.CommitQueueLen, err = Num(v)
			if err != nil {
				errs = append(errs, ArgsErr{k, err.Error()})
			}
		case "cache.write.max-commit-heap-size":
			chdfsConfig.Cache.Write.MaxCommitHeapSize, err = Num(v)
			if err != nil {
				errs = append(errs, ArgsErr{k, err.Error()})
			}
		case "cache.write.auto-merge":
			chdfsConfig.Cache.Write.AutoMerge, err = IsTrue(v)
			if err != nil {
				errs = append(errs, ArgsErr{k, err.Error()})
			}
		case "cache.write.auto-sync":
			chdfsConfig.Cache.Write.AutoSync, err = IsTrue(v)
			if err != nil {
				errs = append(errs, ArgsErr{k, err.Error()})
			}
		case "cache.write.auto-sync-time-ms":
			chdfsConfig.Cache.Write.AutoSyncTimeMs, err = Num(v)
			if err != nil {
				errs = append(errs, ArgsErr{k, err.Error()})
			}
		case "log.level":
			chdfsConfig.Log.Level = v
		case "log.file.filename":
			chdfsConfig.LogFile.Filename = v
		case "log.file.log-rotate":
			chdfsConfig.LogFile.LogRotate, err = IsTrue(v)
			if err != nil {
				errs = append(errs, ArgsErr{k, err.Error()})
			}
		case "log.file.max-size":
			chdfsConfig.LogFile.MaxSize, err = Num(v)
			if err != nil {
				errs = append(errs, ArgsErr{k, err.Error()})
			}
		case "log.file.max-days":
			chdfsConfig.LogFile.MaxDays, err = Num(v)
			if err != nil {
				errs = append(errs, ArgsErr{k, err.Error()})
			}
		case "log.file.max-backups":
			chdfsConfig.LogFile.MaxBackups, err = Num(v)
			if err != nil {
				errs = append(errs, ArgsErr{k, err.Error()})
			}
		default:
			errs = append(errs, ArgsErr{k, "not support"})
		}
	}

	if len(errs) != 0 {
		return nil, fmt.Errorf("invalid argument in additionalArgs, Err: %v", errs)
	}

	return chdfsConfig, nil
}

func IsTrue(str string) (bool, error) {
	isTrue, err := strconv.ParseBool(str)
	if err != nil {
		return false, err
	}
	return isTrue, nil
}

func Num(str string) (int, error) {
	num, err := strconv.Atoi(str)
	if err != nil {
		return 0, err
	}
	return num, nil
}

func WriteFile(file string, context string) error {
	err := os.MkdirAll(path.Dir(file), os.ModePerm)

	writer, err := os.OpenFile(filepath.Clean(file), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer writer.Close()

	_, err = fmt.Fprintln(writer, context)
	return err
}

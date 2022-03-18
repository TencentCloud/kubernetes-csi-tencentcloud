package util

import (
	"os"
	"strings"
	"syscall"

	"github.com/golang/glog"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"k8s.io/utils/mount"
)

const (
	GiB = 1024 * 1024 * 1024
)

type YunApiType string

const (
	CBS YunApiType = "CBS"
	CVM YunApiType = "CVM"
	TAG YunApiType = "TAG"
)

type YunApiAction string

const (
	CreateDisks          YunApiAction = "CreateDisks"
	TerminateDisks       YunApiAction = "TerminateDisks"
	AttachDisks          YunApiAction = "AttachDisks"
	DetachDisks          YunApiAction = "DetachDisks"
	DescribeDisks        YunApiAction = "DescribeDisks"
	DescribeSnapshots    YunApiAction = "DescribeSnapshots"
	DescribeDiskQuota    YunApiAction = "DescribeDiskConfigQuota"
	DescribeInstances    YunApiAction = "DescribeInstances"
	DescribeResourceTags YunApiAction = "DescribeResourceTagsByResourceIds"
)

type PvcAction string

const (
	Provision PvcAction = "provision"
	Delete    PvcAction = "delete"
	Attach    PvcAction = "volume_attach"
	Detach    PvcAction = "volume_detach"
)

type ReturnCode string

const (
	Success        ReturnCode = "Success"
	InternalErr    ReturnCode = "InternalError"
	SdkErrorPrefix ReturnCode = "SDKErr."
	InvalidArgs    ReturnCode = "InvalidArgument"
)

type DiskErrorCode struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

var (
	ErrSnapshotNotFound     = DiskErrorCode{Code: "ErrSnapshotNotFound", Message: "Volume Snapshot not found."}
	ErrInstanceNotFound     = DiskErrorCode{Code: "ErrInstanceNotFound", Message: "CVM instance not found."}
	ErrDiskNotFound         = DiskErrorCode{Code: "ErrDiskNotFound", Message: "CBS disk not found."}
	ErrDiskAttachedAlready  = DiskErrorCode{Code: "ErrDiskAttachedAlready", Message: "CBS disk is attached to another instance already."}
	ErrDiskCreatedTimeout   = DiskErrorCode{Code: "ErrDiskCreatedTimeout", Message: "CBS disk is not ready before deadline exceeded."}
	ErrDiskAttachedTimeout  = DiskErrorCode{Code: "ErrDiskAttachedATimeout", Message: "CBS disk is not attached before deadline exceeded."}
	ErrDiskDetachedTimeout  = DiskErrorCode{Code: "ErrDiskDetachedATimeout", Message: "cbs disk is not unattached before deadline exceeded."}
	ErrDiskTypeNotAvaliable = DiskErrorCode{Code: "ErrDiskTypeSoldOutOrNotSupported", Message: "no available storage in zone(this type is sold out or not supported)."}
)

// RoundUpBytes rounds up the volume size in bytes upto multiplications of GiB
// in the unit of Bytes
func RoundUpBytes(volumeSizeBytes int64) int64 {
	return roundUpSize(volumeSizeBytes, GiB) * GiB
}

// RoundUpGiB rounds up the volume size in bytes upto multiplications of GiB
// in the unit of GiB
func RoundUpGiB(volumeSizeBytes int64) int64 {
	return roundUpSize(volumeSizeBytes, GiB)
}

// BytesToGiB conversts Bytes to GiB
func BytesToGiB(volumeSizeBytes int64) int64 {
	return volumeSizeBytes / GiB
}

// GiBToBytes converts GiB to Bytes
func GiBToBytes(volumeSizeGiB int64) int64 {
	return volumeSizeGiB * GiB
}

// roundUpSize calculates how many allocation units are needed to accommodate
// a volume of given size. E.g. when user wants 1500MiB volume, while AWS EBS
// allocates volumes in gibibyte-sized chunks,
// RoundUpSize(1500 * 1024*1024, 1024*1024*1024) returns '2'
// (2 GiB is the smallest allocatable volume that can hold 1500MiB)
func roundUpSize(volumeSizeBytes int64, allocationUnitBytes int64) int64 {
	roundedUp := volumeSizeBytes / allocationUnitBytes
	if volumeSizeBytes%allocationUnitBytes > 0 {
		roundedUp++
	}
	return roundedUp
}

func GetTencentSdkErrCode(err error) string {
	if sdkError, ok := err.(*errors.TencentCloudSDKError); ok {
		return string(SdkErrorPrefix) + sdkError.Code
	}

	return string(InternalErr)
}

// IsCorruptedMnt return true if err is about corrupted mount point
func IsCorruptedMnt(err error) bool {
	if err == nil {
		return false
	}
	var underlyingError error
	switch pe := err.(type) {
	case nil:
		return false
	case *os.PathError:
		underlyingError = pe.Err
	case *os.LinkError:
		underlyingError = pe.Err
	case *os.SyscallError:
		underlyingError = pe.Err
	}

	return underlyingError == syscall.ENOTCONN || underlyingError == syscall.ESTALE || underlyingError == syscall.EIO || underlyingError == syscall.EACCES
}

// PathExists TODO: clean this up to use pkg/util/file/FileExists
// PathExists returns true if the specified path exists.
func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	} else if os.IsNotExist(err) {
		return false, nil
	} else if IsCorruptedMnt(err) {
		return true, err
	} else {
		return false, err
	}
}

func HasMountRefs(mountPath string, mountRefs []string) bool {
	for _, ref := range mountRefs {
		if !strings.Contains(ref, mountPath) {
			return true
		}
	}
	return false
}

func IsDirMounted(mounter mount.SafeFormatAndMount, mountPath string) (bool, error) {
	notMnt, err := mounter.IsLikelyNotMountPoint(mountPath)
	if err != nil && !os.IsNotExist(err) {
		glog.Error("isDirMounted IsLikelyNotMountPoint test failed for dir [%v]", mountPath)
		return false, err
	}
	return !notMnt, nil
}

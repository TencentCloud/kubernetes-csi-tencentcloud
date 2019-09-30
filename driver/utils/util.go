package utils

import (
	"fmt"
	"os"
	"syscall"

	"github.com/golang/glog"
	"k8s.io/kubernetes/pkg/util/mount"
)

const (
	GiB = 1024 * 1024 * 1024
)

// CleanupMountPoint unmounts the given path and
// deletes the remaining directory if successful.
// if extensiveMountPointCheck is true
// IsNotMountPoint will be called instead of IsLikelyNotMountPoint.
// IsNotMountPoint is more expensive but properly handles bind mounts within the same fs.
func CleanupMountPoint(mountPath string, mounter mount.Interface, extensiveMountPointCheck bool) error {
	// mounter.ExistsPath cannot be used because for containerized kubelet, we need to check
	// the path in the kubelet container, not on the host.
	pathExists, pathErr := PathExists(mountPath)
	if !pathExists {
		glog.Warningf("Warning: Unmount skipped because path does not exist: %v", mountPath)
		return nil
	}
	corruptedMnt := IsCorruptedMnt(pathErr)
	if pathErr != nil && !corruptedMnt {
		return fmt.Errorf("Error checking path: %v", pathErr)
	}
	return doCleanupMountPoint(mountPath, mounter, extensiveMountPointCheck, corruptedMnt)
}

// doCleanupMountPoint unmounts the given path and
// deletes the remaining directory if successful.
// if extensiveMountPointCheck is true
// IsNotMountPoint will be called instead of IsLikelyNotMountPoint.
// IsNotMountPoint is more expensive but properly handles bind mounts within the same fs.
// if corruptedMnt is true, it means that the mountPath is a corrupted mountpoint, and the mount point check
// will be skipped
func doCleanupMountPoint(mountPath string, mounter mount.Interface, extensiveMountPointCheck bool, corruptedMnt bool) error {
	if !corruptedMnt {
		var notMnt bool
		var err error
		if extensiveMountPointCheck {
			notMnt, err = mounter.IsNotMountPoint(mountPath)
		} else {
			notMnt, err = mounter.IsLikelyNotMountPoint(mountPath)
		}

		if err != nil {
			return err
		}

		if notMnt {
			glog.Warningf("Warning: %q is not a mountpoint, deleting", mountPath)
			return os.Remove(mountPath)
		}
	}

	// Unmount the mount path
	glog.Infof("%q is a mountpoint, unmounting", mountPath)
	if err := mounter.Unmount(mountPath); err != nil {
		return err
	}

	notMnt, mntErr := mounter.IsLikelyNotMountPoint(mountPath)
	if mntErr != nil {
		return mntErr
	}
	if notMnt {
		glog.Infof("%q is unmounted, deleting the directory", mountPath)
		return os.Remove(mountPath)
	}
	return fmt.Errorf("Failed to unmount path %v", mountPath)
}

// TODO: clean this up to use pkg/util/file/FileExists
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

// RoundUpGiB rounds up the volume size in bytes upto multiplications of GiB
// in the unit of GiB
func RoundUpGiB(volumeSizeBytes int64) int64 {
	return roundUpSize(volumeSizeBytes, GiB)
}

func roundUpSize(volumeSizeBytes int64, allocationUnitBytes int64) int64 {
	roundedUp := volumeSizeBytes / allocationUnitBytes
	if volumeSizeBytes%allocationUnitBytes > 0 {
		roundedUp++
	}
	return roundedUp
}

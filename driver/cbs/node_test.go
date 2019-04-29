package cbs

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/tencentcloud/kubernetes-csi-tencentcloud/driver/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/kubernetes/pkg/util/mount"
)

func newFakeMounter() *mount.FakeMounter {
	return &mount.FakeMounter{
		MountPoints: []mount.MountPoint{},
		Log:         []mount.FakeAction{},
	}
}

func newFakeSafeFormatAndMounter(fakeMounter *mount.FakeMounter) mount.SafeFormatAndMount {
	return mount.SafeFormatAndMount{
		Interface: fakeMounter,
		Exec:      mount.NewFakeExec(nil),
	}

}

func newFakeCBSNode() *cbsNode {
	return &cbsNode{
		mounter:    newFakeSafeFormatAndMounter(newFakeMounter()),
		idempotent: util.NewIdempotent(),
	}
}

func TestNodeStageVolume(t *testing.T) {
	stdVolCap := &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{},
		},
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		},
	}
	// create fake cbs disk and symlink
	fakeDevicePath := "/dev/fake-cbs-test"
	_, err := os.OpenFile(fakeDevicePath, os.O_RDONLY|os.O_CREATE, 0666)
	if err != nil {
		t.Fatalf("create fakedevice failed %v", err)
	}

	if err := os.Symlink(fakeDevicePath, "/dev/disk/by-id/virtio-disk-test"); err != nil {
		t.Fatalf("create fakedevice symlink failed %v", err)
	}

	defer func() {
		os.Remove(fakeDevicePath)
		os.Remove("/dev/disk/by-id/virtio-disk-test")
	}()

	testCases := []struct {
		name string
		req  *csi.NodeStageVolumeRequest
		// fakeMounter mocks mounter behaviour
		fakeMounter *mount.FakeMounter
		// expected fake mount actions the test will make
		expActions []mount.FakeAction
		// expected test error code
		expErrCode codes.Code
		// expected mount points when test finishes
		expMountPoints []mount.MountPoint
	}{
		{
			name: "success",
			req: &csi.NodeStageVolumeRequest{
				StagingTargetPath: "/test/path",
				VolumeCapability:  stdVolCap,
				VolumeId:          "disk-test",
			},
			fakeMounter: &mount.FakeMounter{
				Filesystem: map[string]mount.FileType{
					fakeDevicePath: mount.FileTypeFile,
				},
			},
			expActions: []mount.FakeAction{
				{
					Action: "mount",
					Target: "/test/path",
					Source: fakeDevicePath,
					FSType: "ext4",
				},
			},
			expMountPoints: []mount.MountPoint{
				{
					Device: fakeDevicePath,
					Opts:   []string{"defaults"},
					Path:   "/test/path",
					Type:   "ext4",
				},
			},
		},
		{
			name: "success mkfs ext3",
			req: &csi.NodeStageVolumeRequest{
				StagingTargetPath: "/test/path",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType: "ext3",
						},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
				VolumeId: "disk-test",
			},
			fakeMounter: &mount.FakeMounter{
				Filesystem: map[string]mount.FileType{
					fakeDevicePath: mount.FileTypeFile,
				},
			},
			expActions: []mount.FakeAction{
				{
					Action: "mount",
					Target: "/test/path",
					Source: fakeDevicePath,
					FSType: "ext3",
				},
			},
			expMountPoints: []mount.MountPoint{
				{
					Device: fakeDevicePath,
					Opts:   []string{"defaults"},
					Path:   "/test/path",
					Type:   "ext3",
				},
			},
		},
		{
			name: "fail no VolumeId",
			req: &csi.NodeStageVolumeRequest{
				StagingTargetPath: "/test/path",
				VolumeCapability:  stdVolCap,
			},
			fakeMounter: newFakeMounter(),
			expErrCode:  codes.InvalidArgument,
		},
		{
			name: "fail no StagingTargetPath",
			req: &csi.NodeStageVolumeRequest{
				VolumeCapability: stdVolCap,
				VolumeId:         "vol-test",
			},
			fakeMounter: newFakeMounter(),
			expErrCode:  codes.InvalidArgument,
		},
		{
			name: "fail no VolumeCapability",
			req: &csi.NodeStageVolumeRequest{
				StagingTargetPath: "/test/path",
				VolumeId:          "vol-test",
			},
			fakeMounter: newFakeMounter(),
			expErrCode:  codes.InvalidArgument,
		},
		{
			name: "success device already mounted at target",
			req: &csi.NodeStageVolumeRequest{
				StagingTargetPath: "/test/path",
				VolumeCapability:  stdVolCap,
				VolumeId:          "disk-test",
			},
			fakeMounter: &mount.FakeMounter{
				MountPoints: []mount.MountPoint{
					{
						Device: fakeDevicePath,
						Path:   "/test/path",
					},
				},
				Filesystem: map[string]mount.FileType{
					fakeDevicePath: mount.FileTypeFile,
				},
			},
			expActions: []mount.FakeAction{},
			expMountPoints: []mount.MountPoint{
				{
					Device: fakeDevicePath,
					Path:   "/test/path",
				},
			},
		},
	}
	cbsNode := newFakeCBSNode()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			_, err := cbsNode.NodeStageVolume(context.TODO(), tc.req)
			if err != nil {
				srvErr, ok := status.FromError(err)
				if !ok {
					t.Fatalf("Could not get error status code from error: %v", srvErr)
				}
				if srvErr.Code() != tc.expErrCode {
					t.Fatalf("Expected error code %d, got %d message %s", tc.expErrCode, srvErr.Code(), srvErr.Message())
				}
			} else if tc.expErrCode != codes.OK {
				t.Fatalf("Expected error %v, got no error", tc.expErrCode)
			}

			if len(tc.fakeMounter.Log) > 0 && !reflect.DeepEqual(tc.fakeMounter.Log, tc.expActions) {
				t.Fatalf("Expected actions {%+v}, got {%+v}", tc.expActions, tc.fakeMounter.Log)
			}
			if len(tc.fakeMounter.MountPoints) > 0 && !reflect.DeepEqual(tc.fakeMounter.MountPoints, tc.expMountPoints) {
				t.Fatalf("Expected mount points {%+v}, got {%+v}", tc.expMountPoints, tc.fakeMounter.MountPoints)
			}
		})
	}
}

func TestNodeUnstageVolume(t *testing.T) {
	testCases := []struct {
		name            string
		req             *csi.NodeUnstageVolumeRequest
		expErrCode      codes.Code
		fakeMountPoints []mount.MountPoint
		// expected fake mount actions the test will make
		expActions []mount.FakeAction
		// expected mount points when test finishes
		expMountPoints []mount.MountPoint
	}{
		{
			name: "success",
			req: &csi.NodeUnstageVolumeRequest{
				StagingTargetPath: "/test/path",
				VolumeId:          "disk-test",
			},
			fakeMountPoints: []mount.MountPoint{
				{Device: "/dev/fake", Path: "/test/path"},
			},
			expActions: []mount.FakeAction{
				{Action: "unmount", Target: "/test/path"},
			},
		},
		{
			name: "success no device mounted at target",
			req: &csi.NodeUnstageVolumeRequest{
				StagingTargetPath: "/test/path",
				VolumeId:          "disk-test",
			},
			expActions: []mount.FakeAction{},
		},
		{
			name: "success device mounted at multiple targets",
			req: &csi.NodeUnstageVolumeRequest{
				StagingTargetPath: "/test/path",
				VolumeId:          "disk-test",
			},
			// mount a fake device in two locations
			fakeMountPoints: []mount.MountPoint{
				{Device: "/dev/fake", Path: "/test/path"},
				{Device: "/dev/fake", Path: "/foo/bar"},
			},
			// it should unmount from the original
			expActions: []mount.FakeAction{
				{Action: "unmount", Target: "/test/path"},
			},
			expMountPoints: []mount.MountPoint{
				{Device: "/dev/fake", Path: "/foo/bar"},
			},
		},
		{
			name: "fail no VolumeId",
			req: &csi.NodeUnstageVolumeRequest{
				StagingTargetPath: "/test/path",
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "fail no StagingTargetPath",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId: "disk-test",
			},
			expErrCode: codes.InvalidArgument,
		},
	}

	cbsNode := newFakeCBSNode()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeMounter := newFakeMounter()
			if len(tc.fakeMountPoints) > 0 {
				cbsNode.mounter.Interface.(*mount.FakeMounter).MountPoints = tc.fakeMountPoints
			}

			_, err := cbsNode.NodeUnstageVolume(context.TODO(), tc.req)
			if err != nil {
				srvErr, ok := status.FromError(err)
				if !ok {
					t.Fatalf("Could not get error status code from error: %v", srvErr)
				}
				if srvErr.Code() != tc.expErrCode {
					t.Fatalf("Expected error code %d, got %d message %s", tc.expErrCode, srvErr.Code(), srvErr.Message())
				}
			} else if tc.expErrCode != codes.OK {
				t.Fatalf("Expected error %v, got no error", tc.expErrCode)
			}
			// if fake mounter did anything we should
			// check if it was expected
			if len(fakeMounter.Log) > 0 && !reflect.DeepEqual(fakeMounter.Log, tc.expActions) {
				t.Fatalf("Expected actions {%+v}, got {%+v}", tc.expActions, fakeMounter.Log)
			}
			if len(fakeMounter.MountPoints) > 0 && !reflect.DeepEqual(fakeMounter.MountPoints, tc.expMountPoints) {
				t.Fatalf("Expected mount points {%+v}, got {%+v}", tc.expMountPoints, fakeMounter.MountPoints)
			}
		})
	}
}

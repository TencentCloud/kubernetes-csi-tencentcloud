package cfsturbo

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	testingexec "k8s.io/utils/exec/testing"
	"k8s.io/utils/mount"
)

func newFakeMounter(mountpoints []mount.MountPoint) *mount.FakeMounter {
	return &mount.FakeMounter{
		MountPoints: mountpoints,
	}
}

func newFakeSafeFormatAndMounter(fakeMounter *mount.FakeMounter) *mount.SafeFormatAndMount {
	fakeExec := &testingexec.FakeExec{ExactOrder: true}
	return &mount.SafeFormatAndMount{
		Interface: fakeMounter,
		Exec:      fakeExec,
	}
}

func newFakeCFSTurboNode(sfm *mount.SafeFormatAndMount) *nodeServer {
	fakeDriver := csicommon.NewCSIDriver(DriverName, DriverVerision, "fakenode")
	fakeDriver.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
	})

	return &nodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(fakeDriver),
		mounter:           sfm,
	}
}

func TestNodeStageVolume(t *testing.T) {
	stdVolCap := &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{},
		},
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
		},
	}
	fakeDevicePath := "10.1.1.1:/"
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
				VolumeId:          "test-volume",
			},
			fakeMounter: &mount.FakeMounter{},
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
					Path:   "/test/path",
					Type:   "ext4",
				},
			},
		},
		{
			name: "success with mount flags",
			req: &csi.NodeStageVolumeRequest{
				StagingTargetPath: "/test/path",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							MountFlags: []string{"soft", "nocto"},
						},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
				VolumeId: "test-volume",
			},
			fakeMounter: &mount.FakeMounter{},
			expActions: []mount.FakeAction{
				{
					Action: "mount",
					Target: "/test/path",
					Source: fakeDevicePath,
				},
			},
			expMountPoints: []mount.MountPoint{
				{
					Device: fakeDevicePath,
					Opts:   []string{"defaults"},
					Path:   "/test/path",
				},
			},
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
	fakeCFSturboNode := newFakeCFSTurboNode(newFakeSafeFormatAndMounter(newFakeMounter([]mount.MountPoint{})))

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			_, err := fakeCFSturboNode.NodeStageVolume(context.TODO(), tc.req)
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

			tcFakeMounterLog := tc.fakeMounter.GetLog()
			if len(tcFakeMounterLog) > 0 && !reflect.DeepEqual(tcFakeMounterLog, tc.expActions) {
				t.Fatalf("Expected actions {%+v}, got {%+v}", tc.expActions, tcFakeMounterLog)
			}
			if len(tc.fakeMounter.MountPoints) > 0 && !reflect.DeepEqual(tc.fakeMounter.MountPoints, tc.expMountPoints) {
				t.Fatalf("Expected mount points {%+v}, got {%+v}", tc.expMountPoints, tc.fakeMounter.MountPoints)
			}
		})
	}
}

func TestNodeUnstageVolume(t *testing.T) {
	fakeTmpStagePath := "/tmp/fakePath"
	_, err := os.OpenFile(fakeTmpStagePath, os.O_RDONLY|os.O_CREATE, 0666)
	if err != nil {
		t.Fatalf("create fakedevice failed %v", err)
	}

	defer func() {
		os.Remove(fakeTmpStagePath)
	}()

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
				VolumeId:          "volume-test",
			},
			expActions: []mount.FakeAction{},
			expErrCode: codes.NotFound,
		},
		{
			name: "success device mounted at multiple targets",
			req: &csi.NodeUnstageVolumeRequest{
				StagingTargetPath: fakeTmpStagePath,
				VolumeId:          "volume-test",
			},
			// mount a fake device in two locations
			fakeMountPoints: []mount.MountPoint{
				{Device: "/dev/fake", Path: fakeTmpStagePath},
				{Device: "/dev/fake", Path: "/foo/bar"},
			},
			// it should unmount from the original
			expActions: []mount.FakeAction{
				{Action: "unmount", Target: fakeTmpStagePath},
			},
			expMountPoints: []mount.MountPoint{
				{Device: "/dev/fake", Path: "/foo/bar"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeMounter := newFakeMounter(tc.fakeMountPoints)
			fakeCFSturboNode := newFakeCFSTurboNode(
				newFakeSafeFormatAndMounter(
					fakeMounter,
				),
			)
			_, err := fakeCFSturboNode.NodeUnstageVolume(context.TODO(), tc.req)
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
			tcFakeMounterLog := fakeMounter.GetLog()
			if len(tcFakeMounterLog) > 0 && !reflect.DeepEqual(tcFakeMounterLog, tc.expActions) {
				t.Fatalf("Expected actions {%+v}, got {%+v}", tc.expActions, tcFakeMounterLog)
			}
			if len(fakeMounter.MountPoints) > 0 && !reflect.DeepEqual(fakeMounter.MountPoints, tc.expMountPoints) {
				t.Fatalf("Expected mount points {%+v}, got {%+v}", tc.expMountPoints, fakeMounter.MountPoints)
			}
		})
	}
}

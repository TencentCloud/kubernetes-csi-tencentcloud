package util

import (
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

const (
	GiB = 1024 * 1024 * 1024
)

type testRequest struct {
	request *csi.CreateVolumeRequest
	expResp bool
	delete  bool
}

var stdVolCap = []*csi.VolumeCapability{
	{
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{},
		},
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		},
	},
}

var (
	stdVolSize  = int64(5 * GiB)
	stdCapRange = &csi.CapacityRange{RequiredBytes: stdVolSize}
	stdParams   = map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
)

func TestInFlight(t *testing.T) {
	testCases := []struct {
		name     string
		requests []testRequest
	}{
		{
			name: "success normal",
			requests: []testRequest{
				{
					request: &csi.CreateVolumeRequest{
						Name:               "random-vol-name",
						CapacityRange:      stdCapRange,
						VolumeCapabilities: stdVolCap,
						Parameters:         stdParams,
					},
					expResp: true,
				},
			},
		},
		{
			name: "success adding request with different name",
			requests: []testRequest{
				{
					request: &csi.CreateVolumeRequest{
						Name:               "random-vol-foobar",
						CapacityRange:      stdCapRange,
						VolumeCapabilities: stdVolCap,
						Parameters:         stdParams,
					},
					expResp: true,
				},
				{
					request: &csi.CreateVolumeRequest{
						Name:               "random-vol-name-foobar",
						CapacityRange:      stdCapRange,
						VolumeCapabilities: stdVolCap,
						Parameters:         stdParams,
					},
					expResp: true,
				},
			},
		},
		{
			name: "success adding request with different parameters",
			requests: []testRequest{
				{
					request: &csi.CreateVolumeRequest{
						Name:               "random-vol-name-foobar",
						CapacityRange:      stdCapRange,
						VolumeCapabilities: stdVolCap,
						Parameters:         map[string]string{"foo": "bar"},
					},
					expResp: true,
				},
				{
					request: &csi.CreateVolumeRequest{
						Name:               "random-vol-name-foobar",
						CapacityRange:      stdCapRange,
						VolumeCapabilities: stdVolCap,
					},
					expResp: true,
				},
			},
		},
		{
			name: "success adding request with different parameters",
			requests: []testRequest{
				{
					request: &csi.CreateVolumeRequest{
						Name:               "random-vol-name-foobar",
						CapacityRange:      stdCapRange,
						VolumeCapabilities: stdVolCap,
						Parameters:         map[string]string{"foo": "bar"},
					},
					expResp: true,
				},
				{
					request: &csi.CreateVolumeRequest{
						Name:               "random-vol-name-foobar",
						CapacityRange:      stdCapRange,
						VolumeCapabilities: stdVolCap,
						Parameters:         map[string]string{"foo": "baz"},
					},
					expResp: true,
				},
			},
		},
		{
			name: "failure adding copy of request",
			requests: []testRequest{
				{
					request: &csi.CreateVolumeRequest{
						Name:               "random-vol-name",
						CapacityRange:      stdCapRange,
						VolumeCapabilities: stdVolCap,
						Parameters:         stdParams,
					},
					expResp: true,
				},
				{
					request: &csi.CreateVolumeRequest{
						Name:               "random-vol-name",
						CapacityRange:      stdCapRange,
						VolumeCapabilities: stdVolCap,
						Parameters:         stdParams,
					},
					expResp: false,
				},
			},
		},
		{
			name: "success add, delete, add copy",
			requests: []testRequest{
				{
					request: &csi.CreateVolumeRequest{
						Name:               "random-vol-name",
						CapacityRange:      stdCapRange,
						VolumeCapabilities: stdVolCap,
						Parameters:         stdParams,
					},
					expResp: true,
				},
				{
					request: &csi.CreateVolumeRequest{
						Name:               "random-vol-name",
						CapacityRange:      stdCapRange,
						VolumeCapabilities: stdVolCap,
						Parameters:         stdParams,
					},
					expResp: false,
					delete:  true,
				},
				{
					request: &csi.CreateVolumeRequest{
						Name:               "random-vol-name",
						CapacityRange:      stdCapRange,
						VolumeCapabilities: stdVolCap,
						Parameters:         stdParams,
					},
					expResp: true,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db := NewInFlight()
			for _, r := range tc.requests {
				var resp bool
				if r.delete {
					db.Delete(r.request)
				} else {
					resp = db.Insert(r.request)
				}
				if r.expResp != resp {
					t.Fatalf("expected insert to be %+v, got %+v", r.expResp, resp)
				}
			}
		})

	}
}

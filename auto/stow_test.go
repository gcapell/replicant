package auto

import (
	"testing"

	"github.com/gcapell/replicant/model"
)

func TestReadyReplicant(t *testing.T) {
	matrix := []model.StowedDevice{{DeviceType: "ftl_relay"}, {DeviceType: "replicant_matrix"}}
	for _, tc := range []struct {
		d    model.Device
		want bool
	}{
		{model.Device{Status: "idle", StowedDevices: matrix}, true},
		{model.Device{Status: "travelling", StowedDevices: matrix}, false},
		{model.Device{Status: "idle", StowedDevices: matrix[:1]}, false},
		{model.Device{Status: "idle", StowedDevices: matrix, AttachedToDeviceCode: "X"}, false},
	} {
		if got := ReadyReplicant(tc.d); got != tc.want {
			t.Errorf("ReadyReplicant(%+v) = %v", tc.d, got)
		}
	}
}

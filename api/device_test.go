package api

import (
	"testing"

	"github.com/gcapell/replicant/model"
)

// A trimmed GET /devices/072FB5E6 response: an autofactory printing, with a queue.
const autofactory = `{
  "attached_to_device_code": null,
  "controller_device_code": null,
  "device_code": "072FB5E6",
  "device_type": "autofactory",
  "features": ["cruise", "print", "modular"],
  "location": "OTHYONAL-BELT-1",
  "print_queue": [
    {"device_type": "ami_survey_controller", "notify": {"device": null}},
    {"device_type": "mining_drone", "notify": {"device": null}},
    {"device_type": "autofactory", "notify": {"device": null}}
  ],
  "printing": {
    "completes_at": "2026-11-08T23:36:58+11:00",
    "device_type": "ami_mining_controller",
    "eta_seconds": 118,
    "progress_percent": 93.4,
    "started_at": "2026-11-08T23:06:58+11:00",
    "tags": []
  },
  "queue_size": 10,
  "replicant_code": "4698E8E6",
  "status": "printing (ami_mining_controller)",
  "stowed_in_device_code": null,
  "tags": []
}`

func TestDecodeAutofactory(t *testing.T) {
	var d model.Device
	if err := unmarshal([]byte(autofactory), &d); err != nil {
		t.Fatal(err)
	}
	p := d.Printing
	if p.DeviceType != "ami_mining_controller" || p.CompletesAt != "2026-11-08T23:36:58+11:00" ||
		p.EtaSeconds != 118 || p.ProgressPercent != 93.4 {
		t.Errorf("printing: %+v", p)
	}
	var queue []string
	for _, q := range d.PrintQueue {
		queue = append(queue, q.DeviceType)
	}
	if len(queue) != 3 || queue[0] != "ami_survey_controller" || queue[2] != "autofactory" {
		t.Errorf("queue: %v", queue)
	}
}

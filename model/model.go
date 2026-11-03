package model

import "time"

type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

type Star struct {
	Designation string   `json:"designation"`
	EntryPoint  string   `json:"entry_point"`
	Explored    bool     `json:"explored"`
	Position    Position `json:"position"`
}

type Stars struct {
	Stars []Star `json:"stars"`
}

type User struct {
	CreatedAt      string              `json:"created_at"`
	Email          string              `json:"email"`
	XPTotal        int                 `json:"experience_points_total"`
	Replicants     []ReplicantLocation `json:"replicants"`
	Status         string              `json:"status"`
	UnreadMessages int
}

type ReplicantLocation struct {
	CurrentLocation string `json:"current_location"`
	CurrentStar     string `json:"current_star"`
	CreatedAt       string `json:"created_at"`
	Code            string `json:"replicant_code"`
}

type StowedDevice struct {
	DeviceCode string `json:"device_code"`
	DeviceType string `json:"device_type"`
}
type LocationDevice struct {
	DeviceCode string `json:"device_code"`
	DeviceType string `json:"device_type"`
	Status     string `json:"status"`
}
type Replicant struct {
	Location      string         `json:"location"`
	Position      Position       `json:"position"`
	PrintQueue    []string       `json:"print_queue"`
	Status        string         `json:"status"`
	StowedDevices []StowedDevice `json:"stowed_devices"`
	Items         map[string]int `json:"unread_message_count"`
}

type Inventory struct {
	Locations []InventoryLocation `json:"locations"`
}

type InventoryLocation struct {
	Location string          `json:"location"`
	Items    []InventoryItem `json:"items"`
}

type InventoryItem struct {
	ResourceType string `json:"resource_type"`
	Quantity     int    `json:"quantity"`
}

const (
	miningDrone = "mining_drone"
	surveyDrone = "survey_drone"
)

type Command struct {
	Command      string `json:"command"`
	ResourceType string `json:"resource_type,omitempty"`
}

func deployCmd() *Command {
	return &Command{Command: "deploy"}
}

type Location struct {
	Belt          Belt             `json:"belt"`
	Inventory     []InventoryItem  `json:"inventory"`
	Devices       []LocationDevice `json:"devices"`
	ResourceSites []ResourceSite   `json:"resource_sites"`
}

type ResourceSite struct {
	Name      string         `json:"name"`
	Remaining map[string]int `json:"resources_remaining_pct"`
}

type Belt struct {
	Resources map[string]string `json:"resources"`
}

type SearchResponse struct {
	CompletesAt  string  `json:"completes_at"`
	EtaSeconds   float64 `json:"eta_seconds"`
	SearchTarget string  `json:"search_target"`
	SearchType   string  `json:"search_type"`
}

type MiningResponse struct {
	Availability     string `json:"availability"`
	Belt             string `json:"belt"`
	CycleTimeSeconds int    `json:"cycle_time_seconds"`
	Density          string `json:"density"`
	Designation      string `json:"designation"`
	Location         string `json:"location"`
	LocationType     string `json:"location_type"`
	ResourceType     string `json:"resource_type"`
	SalvageType      string `json:"salvage_type"`
	Site             string `json:"site"`
}

// Payload for "ami.mining.digest"
type MiningDigest struct {
	Directive string `json:"directive"`
	Activity  struct {
		Counts     map[string]int `json:"counts"`
		EventCount int            `json:"event_count"`
		Window     [2]time.Time   `json:"window"`
	} `json:"activity"`
	Devices []struct {
		DeviceCode string `json:"device_code"`
		Status     string `json:"status"`
		Events     int    `json:"events,omitempty"`
		LastEvent  string `json:"last_event,omitempty"`
	} `json:"devices"`
	Report struct {
		Location  string                   `json:"location"`
		Resources map[string]ResourceLevel `json:"resources"` // keyed by carbon, rares, ...
	} `json:"report"`
}

type ResourceLevel struct {
	Actual   int `json:"actual"`
	Capacity int `json:"capacity"`
	Desired  int `json:"desired"`
}

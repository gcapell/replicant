package main

const (
	Token = "J9RbhJ2aoZhOInX5j6lBUUH1l8oO4aKHPcSbA94OeacU9BRuRBiV-2dk1TsXa9je"

	Code        = "4698E8E6"
	APIEndpoint = "https://api.replicant.space/v1"
)

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

type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
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
	Command string `json:"command"`
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
	CompletesAt  string `json:"completes_at"`
	EtaSeconds   int    `json:"eta_seconds"`
	SearchTarget string `json:"search_target"`
	SearchType   string `json:"search_type"`
}

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
	UnreadMessages int                 `json:"unread_message_count"`
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
type Replicant struct {
	Location      string         `json:"current_location"`
	Position      Position       `json:"position"`
	PrintQueue    []string       `json:"print_queue"`
	Status        string         `json:"status"`
	StowedDevices []StowedDevice `json:"stowed_devices"`
}

const (
	miningDrone = "mining_drone"
)

type Command struct {
	Command string `json:"command"`
}

func deployCmd() *Command {
	return &Command{Command: "deploy"}
}

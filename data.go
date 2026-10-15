package main

const (
	Token = "J9RbhJ2aoZhOInX5j6lBUUH1l8oO4aKHPcSbA94OeacU9BRuRBiV-2dk1TsXa9je"

	Code        = "4698E8E6"
	APIEndpoint = "https://api.replicant.space/v1"
)

type User struct {
	CreatedAt      string      `json:"created_at"`
	Email          string      `json:"email"`
	XPTotal        int         `json:"experience_points_total"`
	Replicants     []Replicant `json:"replicants"`
	Status         string      `json:"status"`
	UnreadMessages int         `json:"unread_message_count"`
}

type Replicant struct {
	CurrentLocation string `json:"current_location"`
	CurrentStar     string `json:"current_star"`
	CreatedAt       string `json:"created_at"`
	Code            string `json:"replicant_code"`
}

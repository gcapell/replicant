package model

type Paged struct {
	NextCursor *int
}

func (p Paged) Cursor() (int, bool) {
	if p.NextCursor == nil {
		return 0, false
	}
	return *p.NextCursor, true
}

type Pageable interface {
	Cursor() (int, bool)
}

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

// SystemScan is the part of a system scan response we use.
type SystemScan struct {
	EntryPoint string `json:"entry_point"`
}

// ProspectStarted is the response to a galactic_observatory's prospect command.
type ProspectStarted struct {
	Status      string `json:"status"`
	CompletesAt string `json:"completes_at"`
}

type Stars struct {
	Stars []Star `json:"stars"`
}

type User struct {
	CreatedAt          string              `json:"created_at"`
	Email              string              `json:"email"`
	XPTotal            int                 `json:"experience_points_total"`
	Replicants         []ReplicantLocation `json:"replicants"`
	Status             string              `json:"status"`
	UnreadMessageCount int
}

type ReplicantLocation struct {
	CurrentLocation string `json:"current_location"`
	CurrentStar     string `json:"current_star"`
	CreatedAt       string `json:"created_at"`
	Code            string `json:"replicant_code"`
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

type Command struct {
	Command      string `json:"command"`
	ResourceType string `json:"resource_type,omitempty"`
}

// Location is the response to GET /locations/{code}. Which fields are filled
// depends on the kind of location: the star fields below come from a star
// code (e.g. URIRALAX).
type Location struct {
	Location     string `json:"location"`
	LocationType string `json:"location_type"`

	Belt          Belt             `json:"belt"`
	Inventory     []InventoryItem  `json:"inventory"`
	Devices       []LocationDevice `json:"devices"`
	ResourceSites []ResourceSite   `json:"resource_sites"`
	LocationEvent *LocationEvent   `json:"location_event"`

	// Stars only.
	ActiveLocationEvents []any          `json:"active_location_events"`
	AsteroidBelt         AsteroidBelt   `json:"asteroid_belt"`
	EntryPoint           string         `json:"entry_point"`
	LifeDetected         bool           `json:"life_detected"`
	MiningBonusPct       float64        `json:"mining_bonus_pct"`
	MoonsScanned         int            `json:"moons_scanned"`
	MoonsTotal           int            `json:"moons_total"`
	MoonsTotalEstimated  bool           `json:"moons_total_estimated"`
	OuterSystem          OuterSystem    `json:"outer_system"`
	Planets              []SystemPlanet `json:"planets"`
	PlanetsScanned       int            `json:"planets_scanned"`
	PlanetsTotal         int            `json:"planets_total"`
	Shops                []any          `json:"shops"`
	Star                 StarDetails    `json:"star"`
	SystemObjects        []any          `json:"system_objects"`
	SystemScanned        bool           `json:"system_scanned"`
	SystemTags           []string       `json:"system_tags"`
}

// Unscanned is how many planets and moons in a star's system haven't been
// scanned yet.
func (l Location) Unscanned() int {
	return l.PlanetsTotal - l.PlanetsScanned + l.MoonsTotal - l.MoonsScanned
}

type AsteroidBelt struct {
	Present bool `json:"present"`
}

type OuterSystem struct {
	Kuiper *OuterBody `json:"kuiper"`
	Oort   *OuterBody `json:"oort"`
}

type OuterBody struct {
	Designation string  `json:"designation"`
	DistanceAU  float64 `json:"distance_au"`
}

// SystemPlanet is one planet in a star's GET /locations response.
type SystemPlanet struct {
	Designation        string          `json:"designation"`
	InHabitableZone    bool            `json:"in_habitable_zone"`
	Inventory          []InventoryItem `json:"inventory"`
	LifeEstimated      bool            `json:"life_estimated"`
	LifeStage          string          `json:"life_stage"`
	MoonCount          int             `json:"moon_count"`
	MoonCountEstimated bool            `json:"moon_count_estimated"`
	Name               *string         `json:"name"`
	OrbitalDistanceAU  float64         `json:"orbital_distance_au"`
	Salvage            []Salvage       `json:"salvage"`
	Scanned            bool            `json:"scanned"`
	Type               string          `json:"type"`
	TypeEstimated      bool            `json:"type_estimated"`
}

type Salvage struct {
	Depleted           bool     `json:"depleted"`
	Designation        string   `json:"designation"`
	Location           string   `json:"location"`
	Name               string   `json:"name"`
	ResourcesAvailable []string `json:"resources_available"`
	SalvageType        string   `json:"salvage_type"`
}

type StarDetails struct {
	AgeMY           float64       `json:"age_my"`
	Color           string        `json:"color"`
	Designation     string        `json:"designation"`
	DistanceFromSol float64       `json:"distance_from_sol"`
	HabitableZone   HabitableZone `json:"habitable_zone"`
	LuminositySolar float64       `json:"luminosity_solar"`
	MassSolar       float64       `json:"mass_solar"`
	MiningBonusPct  float64       `json:"mining_bonus_pct"`
	Name            *string       `json:"name"`
	Position        Position      `json:"position"`
	RadiusSolar     float64       `json:"radius_solar"`
	SpectralType    string        `json:"spectral_type"`
	StellarClass    string        `json:"stellar_class"`
	TemperatureK    float64       `json:"temperature_k"`
}

type HabitableZone struct {
	InnerAU float64 `json:"inner_au"`
	OuterAU float64 `json:"outer_au"`
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

type ResourceLevel struct {
	Actual   int `json:"actual"`
	Capacity int `json:"capacity"`
	Desired  int `json:"desired"`
}

// LocationList is the response to GET /locations: a summary of every known point of interest.
type LocationList struct {
	Locations map[string]LocationSummary `json:"locations"`
}

type LocationSummary struct {
	Devices        int `json:"devices"`
	LocationEvents int `json:"location_events"`
	Replicants     int `json:"replicants"`
	ResourceSites  int `json:"resource_sites"`
	Resources      int `json:"resources"`
}

// InventoryList is one page of GET /inventory: stockpiles per location.
// Unlike most lists, its cursor is a location code, not an integer.
type InventoryList struct {
	Locations  []LocationInventory `json:"locations"`
	NextCursor *string             `json:"next_cursor"`
}

type LocationInventory struct {
	Location string          `json:"location"`
	Items    []InventoryItem `json:"items"`
}

// Amounts returns the location's stockpile as resource type -> quantity.
func (l LocationInventory) Amounts() map[ResourceType]int {
	m := make(map[ResourceType]int, len(l.Items))
	for _, it := range l.Items {
		m[ResourceType(it.ResourceType)] += it.Quantity
	}
	return m
}

// Total is the sum of all resources at the location.
func (l LocationInventory) Total() int {
	n := 0
	for _, it := range l.Items {
		n += it.Quantity
	}
	return n
}

// AccountEvents is one page of GET /accounts/events.
type AccountEvents struct {
	Events []LocationEvent `json:"events"`
	Paged
}

// LocationEvent is a request from a location's inhabitants, as listed by
// GET /accounts/events (and GET /locations/{code}'s location_event).
//
// An option's (and progress's) met stays false until a replicant is
// present, even when every resource and device in it is met; see
// EventOption.Satisfied.
type LocationEvent struct {
	Location    string        `json:"location"`
	Status      string        `json:"status"`
	Designation string        `json:"designation"`
	Title       string        `json:"title"`
	EventType   string        `json:"event_type"`
	Tier        int           `json:"tier"`
	Progress    EventProgress `json:"progress"`
}

// EventProgress lists the alternative ways an event can be met.
type EventProgress struct {
	Options []EventOption `json:"options"`
}

type EventOption struct {
	Name      string             `json:"name"`
	Met       bool               `json:"met"`
	Resources []RequiredResource `json:"resources"`
	Devices   []RequiredDevice   `json:"devices"`
}

// Satisfied reports whether every resource and device the option asks for
// is at the location (Met also needs a replicant present).
func (o EventOption) Satisfied() bool {
	for _, r := range o.Resources {
		if r.Current < r.Required {
			return false
		}
	}
	for _, d := range o.Devices {
		if d.Current < d.Required {
			return false
		}
	}
	return true
}

type RequiredResource struct {
	ResourceType ResourceType `json:"resource_type"`
	Current      int          `json:"current"`
	Required     int          `json:"required"`
	Met          bool         `json:"met"`
}

type RequiredDevice struct {
	DeviceType string `json:"device_type"`
	Current    int    `json:"current"`
	Required   int    `json:"required"`
	Met        bool   `json:"met"`
}

// CompletedEvent is the part of POST /locations/{loc}/events/{designation}'s
// response we use.
type CompletedEvent struct {
	Designation string `json:"designation"`
	Title       string `json:"title"`
	Status      string `json:"status"`
}

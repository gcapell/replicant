package model

import (
	jsoniter "github.com/json-iterator/go"
)

type Decoder func([]byte) (any, error)

var EventRegistry = map[string]Decoder{}

type NewReplicant struct {
	HostDeviceCode   string
	NewReplicantCode string
	NewReplicantName string
}
type AMIDigest struct {
	Directive string
	Activity  AMIActivity
	Devices   []AMIDevice
}
type AMIActivity struct {
	Counts     map[string]int
	EventCount int
	Window     [2]string
}
type AMIDevice struct {
	DeviceCode string
	Status     string
	Events     int
	LastEvent  string
}

// Payload for "ami.mining.digest"
type MiningDigest struct {
	AMIDigest
	Report MiningReport
}

type MiningReport struct {
	Location  string
	Resources map[ResourceType]ResourceLevel
}

type SurveyDigest struct {
	AMIDigest
	Report SurveyReport
}

type SurveyReport struct {
	ActiveSites             int
	Belt                    string
	Cruising                int
	Busy                    int
	Progress                SurveyProgress
	Idle                    int
	MaxSites                int
	Scans                   []SurveyScan
	Searching               int
	TotalResourcesAvailable int
}
type SurveyProgress struct {
	Remaining int
	Scanned   int
	Total     int
}
type SurveyScan struct {
	DeviceCode string
	Report     SurveyScanReport
}
type SurveyScanReport struct {
	Moons  []Moon
	Planet Planet
}
type Planet struct {
	Designation     string
	InHabitableZone bool
	LifeStage       string
	Name            string
	SpeciesName     string
	Tags            []string
}
type Moon struct {
	Designation string
	Name        string
	Scanned     bool
	Type        string
}
type ExperienceGained struct {
	Amount int
	Source string
}

type TransportCommon struct {
	CargoAfter    int
	CargoCapacity int
	Resources     map[ResourceType]float64
	Total         float64
}

type TransportCollected struct {
	TransportCommon
}

type TransportDelivered struct {
	TransportCommon
}

type TravelDeparted struct {
	ArrivesAt         string
	Origin            string
	Destination       string
	TravelTimeSeconds float64
	TravelType        string
	Legs              []TravelLeg
}
type TravelLeg struct {
	To   string
	Type string
}

type TravelArrived struct {
	Destination string
	Origin      string
	Recalling   bool
	TravelType  string
}

type BobNetNew struct {
	Channel       string
	CurrentStar   string
	ID            int
	Message       string
	ReplicantCode string
	ReplicantName string
}

type PrintStarted struct {
	CompletesAt string
	DeviceType  string
	PrintMode   string
}

type MessageNew struct {
	Body        string
	MessageId   int
	MessageType string
	Title       string
}
type EventDiscovered struct {
	Category    string
	Criteria    []EventCriterion
	Description string
	Designation string
	EventType   string
	Location    string
	Rewards     EventRewards
	Tier        int
	Title       string
}
type EventCriterion struct {
	Name      string
	Devices   []EventDevice
	Resources map[ResourceType]int
}

type EventCompleted struct {
	Consumed    EventRewards
	Designation string
	EventType   string
	Location    string
	Rewards     EventRewards
	Tier        int
}

type EventRewards struct {
	Resources          map[ResourceType]int
	Devices            []EventDevice
	CivilisationPoints int
	XP                 int
}
type EventDevice struct {
	DeviceCode string
	DeviceType string
}

type PrintCompleted struct {
	Compacted     bool
	DeviceType    string
	NewDeviceCode string
	PrintMode     string
}

type SiteDepleted struct {
	Site string
}

type DeviceAttached struct {
	TargetCode string
	TargetType string
}

type DeviceDetached struct {
	TargetCode string
	TargetType string
}
type DeviceUnfurling struct {
	CompletesAt string
}
type DeviceUnfurled struct{}
type DeviceCompacted struct{}

type ProspectCompleted struct {
	Origin         string
	Stars          []string
	StarsGenerated int
}
type DeviceCompacting struct {
	CompletesAt string
}
type DirectiveSet struct {
	Configuration DirectiveConfiguration
	Directive     string
}

type SurveyDirectiveConfiguration struct {
	Moons   string
	Planets string
	Recall  bool
}
type DirectiveConfiguration struct {
	SurveyDirectiveConfiguration
}

type DeviceStowed struct {
	StowedInDeviceCode string
}
type DeviceDeployed struct {
	DeployedFrom string
}

type RelayActivated struct {
	Location string
	Star     string
}
type DirectiveCompleted struct {
	Directive string
}
type SalvageDiscovered struct {
	Designation string
	Location    string
	Name        string
	Resources   map[ResourceType]int
	SalvageType string
}

var unmarshal = jsoniter.ConfigCompatibleWithStandardLibrary.Unmarshal

func register[T any](name string) {
	EventRegistry[name] = func(raw []byte) (any, error) {
		var v T
		if err := unmarshal(raw, &v); err != nil {
			return nil, err
		}
		return &v, nil
	}
}

func init() {
	register[NewReplicant]("story.awakened")
	register[MiningDigest]("ami.mining.digest")
	register[SurveyDigest]("ami.survey.digest")
	register[ExperienceGained]("experience.gained")
	register[TransportCollected]("transport.collected")
	register[TransportDelivered]("transport.delivered")
	register[TravelDeparted]("travel.departed")
	register[TravelArrived]("travel.arrived")
	register[BobNetNew]("bobnet.new")
	register[PrintStarted]("print.started")
	register[MessageNew]("message.new")
	register[EventCompleted]("event.completed")
	register[PrintCompleted]("print.completed")
	register[SiteDepleted]("site.depleted")
	register[DeviceAttached]("device.attached")
	register[DeviceDetached]("device.detached")
	register[DeviceUnfurling]("device.unfurling")
	register[DeviceUnfurled]("device.unfurled")
	register[ProspectCompleted]("prospect.completed")
	register[DeviceCompacting]("device.compacting")
	register[DeviceCompacted]("device.compacted")
	register[DirectiveSet]("directive.set")
	register[EventDiscovered]("event.discovered")
	register[DeviceStowed]("device.stowed")
	register[DeviceDeployed]("device.deployed")
	register[RelayActivated]("relay.activated")
	register[DirectiveCompleted]("directive.completed")
	register[SalvageDiscovered]("salvage.discovered")
}

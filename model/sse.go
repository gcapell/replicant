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
	ActiveStites            int
	Belt                    string
	Cruising                int
	Idle                    int
	MaxSites                int
	Scans                   []string
	Searching               int
	TotalResourcesAvailable int
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

type EventCompleted struct {
	Consumed    EventStakes
	Designation string
	EventType   string
	Location    string
	Rewards     EventStakes
	Tier        int
}

type EventStakes struct {
	Resources          map[ResourceType]int
	CivilisationPoints int
	XP                 int
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
}

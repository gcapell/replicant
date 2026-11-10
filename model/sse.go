package model

import (
	jsoniter "github.com/json-iterator/go"
	"github.com/json-iterator/go/extra"
)

// JSON field names are derived from Go names (CargoCapacity ->
// cargo_capacity), for API responses and event payloads alike. This is set
// here, where the event decoders live, so that they work in any package that
// imports model, not only those that also import api.
func init() {
	extra.SetNamingStrategy(extra.LowerCaseWithUnderscores)
}

type Decoder func([]byte) (any, error)

var EventRegistry = map[string]Decoder{}

// --- ami ---

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

// SurveyScan is one scan in a survey digest's report. Not every scan is
// listed: a digest's activity can count more scan.completed than it lists.
type SurveyScan struct {
	DeviceCode string
	ScanTarget string // e.g. CORQUELAI-5
	ScanType   string // planet or moon
	Report     SurveyScanReport
}

// SurveyScanReport is a planet scan's Planet and its Moons (not yet scanned
// themselves), or a moon scan's Moon.
type SurveyScanReport struct {
	Moons  []Moon
	Planet Planet
	Moon   ScannedMoon
}
type Planet struct {
	Designation     string
	InHabitableZone bool
	LifeStage       string
	Name            string
	SpeciesName     string
	Tags            []string
	Type            string // e.g. dwarf
}

// ScannedMoon is a moon scan's report (the fields we use).
type ScannedMoon struct {
	Designation string
	LifeStage   string
	Type        string // e.g. icy
}
type Moon struct {
	Designation string
	Name        string
	Scanned     bool
	Type        string
}
type AMIAdopted struct {
	Devices []EventDevice
}
type AMIReleased struct {
	Devices []EventDevice
}
type AMIAssembling struct {
	Destination     string
	AssemblingCount int
}
type AMILaunched struct {
	DirectiveStatus string
	Evaluated       bool
	DevicesDeployed int
}
type AMIWithdrawn struct {
	DirectivePaused bool
	DevicesRecalled int
}

// --- blueprint ---

type BlueprintUnlocked struct {
	DeviceType          string
	ShortDescription    string
	Description         string
	Resources           map[ResourceType]int
	PrintTime           int
	RequiresAutofactory bool
	Source              string
}

// --- bobnet ---

type BobNetNew struct {
	Channel       string
	CurrentStar   string
	Id            int
	Message       string
	ReplicantCode string
	ReplicantName string
}

// --- device ---

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
type DeviceCompacting struct {
	CompletesAt string
}
type DeviceStowed struct {
	StowedInDeviceCode string
}
type DeviceDeployed struct {
	DeployedFrom string
}
type DeviceChangedOwner struct {
	FromReplicant string
	ToReplicant   string
	Direction     string
}
type DeviceDecommissioned struct {
	ResourcesRecovered map[ResourceType]int
}
type DeviceDestroyed struct{}
type DeviceLost struct{}
type DeviceDiscovered struct {
	DeviceType string
	DeviceCode string
}

// --- directive ---

type DirectiveSet struct {
	Configuration DirectiveConfiguration
	Directive     string
}
type SurveyDirectiveConfiguration struct {
	Moons   string
	Planets string
	Recall  bool
}

// Fields from all directive types; only those relevant to Directive are set.
type DirectiveConfiguration struct {
	SurveyDirectiveConfiguration
	MiningDirectiveConfiguration
}
type MiningDirectiveConfiguration struct {
	Resource ResourceType
	Desired  int
}
type DirectiveCompleted struct {
	Directive string
}
type DirectiveCleared struct {
	PreviousDirective string
}
type DirectivePaused struct {
	Directive string
}
type DirectiveResumed struct {
	Directive string
}

// DirectiveResumeFailure is a directive failing to resume: Command is what it
// tried, e.g. {"command": "start_mining", "resource_type": "rares"}.
type DirectiveResumeFailure struct {
	Command map[string]any
	Reason  string // e.g. "start_mining failed: No salvage available at this location"
}

// --- diversion ---

type DiversionActivated struct {
	ObjectDesignation string
	SizeClass         string
}
type DiversionDeactivated struct {
	DeviceCode string
}

// --- event ---

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
	Xp                 int
}
type EventDevice struct {
	DeviceCode string
	DeviceType string
}

// --- experience ---

type ExperienceGained struct {
	Amount int
	Source string
}

// --- hub ---

type HubActivated struct {
	Star     string
	Location string
}
type HubDestroyed struct {
	Star     string
	Location string
}
type HubWarning struct {
	Capacity    float64
	WarningType string
}
type HubMaintained struct {
	ResourcesConsumed map[ResourceType]int
	Capacity          float64
}

// --- megastructure ---

type MegastructureContributed struct {
	MegastructureDesignation string
	AcceptedCount            int
	ContributedDevices       []EventDevice
}

// --- message ---

type MessageNew struct {
	Body        string
	MessageId   int
	MessageType string
	Title       string
}

// --- mining ---

type MiningRelocated struct {
	Location     string
	ResourceType ResourceType
	OldSite      string
	NewSite      string
}
type MiningRetargeted struct {
	LocationType     string
	Location         string
	Site             string
	OldResource      ResourceType
	NewResource      ResourceType
	Availability     string
	Density          string
	CycleTimeSeconds int
}
type MiningStarted struct {
	LocationType     string
	Location         string
	Site             string
	ResourceType     ResourceType
	Availability     string
	Density          string
	CycleTimeSeconds int
}
type MiningStopped struct {
	Location      string
	ResourceType  ResourceType
	QuantityMined int
}

// --- multiplayer ---

type ReplicantRef struct {
	ReplicantCode string
	ReplicantName string
}
type ReplicantEntered struct {
	ReplicantRef
}
type ReplicantLeft struct {
	ReplicantRef
}

// --- print ---

type PrintStarted struct {
	CompletesAt string
	DeviceType  string
	PrintMode   string
}
type PrintCompleted struct {
	Compacted     bool
	DeviceType    string
	NewDeviceCode string
	PrintMode     string
}
type PrintCancelled struct {
	DeviceType          string
	PrintMode           string
	ReturnedDeviceCodes []string
}

// --- prospect ---

type ProspectCompleted struct {
	Origin         string
	Stars          []string
	StarsGenerated int
}

// --- relay ---

type RelayActivated struct {
	Location string
	Star     string
}

// --- replicant ---

type ReplicantTransferred struct {
	OldHost string
	NewHost string
}

// --- salvage ---

type SalvageDiscovered struct {
	Designation string
	Location    string
	Name        string
	Resources   map[ResourceType]int
	SalvageType string
}
type SalvageDepleted struct {
	Site string
}

// --- scan ---

type ScanStarted struct {
	ScanTarget  string
	ScanType    string
	EtaSeconds  int
	CompletesAt string
}
type ScanCompleted struct {
	ScanTarget string
	ScanType   string
	Report     map[string]ScanReport // keyed by scan type: belt, planet, moon
}
type ScanReport struct {
	Density       string
	Designation   string
	ResourceSites []ScanResourceSite
}
type ScanResourceSite struct {
	Designation string
	Name        string
	Resources   map[ResourceType]ScanResource
	SiteIndex   int
	TrackedBy   string
}
type ScanResource struct {
	Availability string
	DepletionPct int
	Original     int
	Remaining    int
}

// --- search ---

type SearchStarted struct {
	SearchTarget string
	SearchType   string
	EtaSeconds   int
	CompletesAt  string
}
type SearchCompleted struct {
	SearchTarget string
	SearchType   string
	Report       SearchReport
}
type SearchReport struct {
	Site      string
	Resources map[ResourceType]int
}

// --- simulation ---

type SimulationRef struct {
	SimulationId int
	ScenarioCode string
}
type SimulationStarted struct {
	SimulationRef
	StartingStar string
}
type SimulationCompleted struct {
	SimulationRef
	ScoreSeconds   int
	ResourcesMined int
	DevicesPrinted int
}
type SimulationAbandoned struct {
	SimulationRef
}
type SimulationExpired struct {
	SimulationRef
}

// --- site ---

type SiteDepleted struct {
	Site string
}

// --- story ---

type NewReplicant struct {
	HostDeviceCode   string
	NewReplicantCode string
	NewReplicantName string
}
type StoryHint struct {
	Hint        string
	Planet      string
	Designation string
}

// --- system ---

type SystemBodyRenamed struct {
	BodyType    string
	Designation string
	NewName     string
}
type SystemDevicesHalted struct {
	Star          string
	DevicesHalted int
}
type SystemEntryPointSet struct {
	Star       string
	EntryPoint string
}
type SystemImpactWarning struct {
	Destination string
	ImpactEta   string
	DeviceCodes []string
}
type SystemObjectDetected struct {
	ObjectDesignation string
	SizeClass         string
	ImpactTarget      string
	ImpactEta         string
	DiscoverySource   string
}
type SystemObjectDiverted struct {
	ObjectDesignation string
	Outcome           string
	ImpactTarget      string
	MassClass         string
}
type SystemObjectImpacted struct {
	ObjectDesignation string
	ImpactTarget      string
	MassClass         string
	Composition       string
	ApproachSpeed     float64
	ApproachAngle     float64
	Description       string
	Depleted          bool
	TerraformEffects  map[string]float64
}
type SystemObjectOffTarget struct {
	ObjectDesignation string
	ImpactTarget      string
	MassClass         string
	NewStatus         string
}

// --- teleport ---

type TeleportStarted struct {
	SourceStar       string
	DestinationStar  string
	TargetMatrixCode string
}
type TeleportCompleted struct {
	DestinationStar string
	NewHostCode     string
}
type TeleportFailed struct {
	Reason           string
	TargetMatrixCode string
}

// --- terraform ---

type TerraformActivated struct{}
type TerraformDeactivated struct{}
type TerraformDeviceActivated struct{}
type TerraformDeviceDeactivated struct{}
type TerraformStatus struct {
	Body        string
	Attributes  map[string]TerraformAttribute
	Projections map[string]map[string]float64 // attribute -> "+6"/"+12"/"+24" -> value
	Effects     TerraformEffects
	Warnings    []string
	Anomaly     any
	Asteroids   []any
}
type TerraformAttribute struct {
	Value float64
	Delta float64
}
type TerraformEffects struct {
	Devices     map[string]TerraformDeviceEffects
	Equilibrium []TerraformEffect
	Cascade     []TerraformEffect
}
type TerraformDeviceEffects struct {
	DeviceType string
	Effects    []TerraformEffect
}
type TerraformEffect struct {
	Attribute string
	Direction string
	Strength  string
}

// --- trade ---

type TradeGoods struct {
	Resources map[ResourceType]int
	Devices   []EventDevice
}
type TradeCompleted struct {
	TradeCode        string
	TradeName        string
	Role             string // "buyer" or "seller"
	RemainingStock   int
	CriteriaPaid     TradeGoods // buyer
	RewardsReceived  TradeGoods // buyer
	CriteriaReceived TradeGoods // seller
}
type TradeCreated struct {
	TradeCode string
	Name      string
	Stock     int
	Escrowed  TradeGoods
}
type TradeDeleted struct {
	TradeCode      string
	Name           string
	RemainingStock int
	Released       TradeGoods
}

// --- transport ---

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

// --- travel ---

type TravelDeparted struct {
	ArrivesAt         string // seen on single-leg trips; may be missing on multi-leg ones
	Origin            string
	Destination       string // the trip's final destination
	TravelTimeSeconds float64
	TravelType        string
	Legs              []TravelLeg
	AttachedDevices   []string // devices carried along
}
type TravelLeg struct {
	To   string
	Type string
}
type TravelArrived struct {
	Destination     string // a bare star after a surge; the envelope's Location is exact
	Origin          string
	Recalling       bool
	TravelType      string
	AttachedDevices []string // devices carried along
}
type TravelCancelled struct {
	TravelType        string
	Origin            string
	Destination       string
	ReturnTimeSeconds int
	AttachedDevices   []string
}

// --- triangulation ---

type TriangulationStarted struct {
	Signature   string
	Target      [3]float64
	CompletesAt string
}
type TriangulationComplete struct {
	Signature string
	Target    [3]float64
	Direction [3]float64
}
type TriangulationFailed struct {
	Signature string
	Target    [3]float64
	Reason    string
}

// --- ward ---

type WardActivated struct{}
type WardDeactivated struct{}

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
	register[AMIAdopted]("ami.adopted")
	register[AMIAssembling]("ami.assembling")
	register[AMILaunched]("ami.launched")
	register[MiningDigest]("ami.mining.digest")
	register[AMIReleased]("ami.released")
	register[SurveyDigest]("ami.survey.digest")
	register[AMIWithdrawn]("ami.withdrawn")

	register[BlueprintUnlocked]("blueprint.unlocked")

	register[BobNetNew]("bobnet.new")

	register[DeviceAttached]("device.attached")
	register[DeviceChangedOwner]("device.changed_owner")
	register[DeviceCompacted]("device.compacted")
	register[DeviceCompacting]("device.compacting")
	register[DeviceDecommissioned]("device.decommissioned")
	register[DeviceDeployed]("device.deployed")
	register[DeviceDestroyed]("device.destroyed")
	register[DeviceDetached]("device.detached")
	register[DeviceDiscovered]("device.discovered")
	register[DeviceLost]("device.lost")
	register[DeviceStowed]("device.stowed")
	register[DeviceUnfurled]("device.unfurled")
	register[DeviceUnfurling]("device.unfurling")

	register[DirectiveCleared]("directive.cleared")
	register[DirectiveCompleted]("directive.completed")
	register[DirectivePaused]("directive.paused")
	register[DirectiveResumed]("directive.resumed")
	register[DirectiveResumeFailure]("directive.resume_failure")
	register[DirectiveSet]("directive.set")

	register[DiversionActivated]("diversion.activated")
	register[DiversionDeactivated]("diversion.deactivated")

	register[EventCompleted]("event.completed")
	register[EventDiscovered]("event.discovered")

	register[ExperienceGained]("experience.gained")

	register[HubActivated]("hub.activated")
	register[HubDestroyed]("hub.destroyed")
	register[HubMaintained]("hub.maintained")
	register[HubWarning]("hub.warning")

	register[MegastructureContributed]("megastructure.contributed")

	register[MessageNew]("message.new")

	register[MiningRelocated]("mining.relocated")
	register[MiningRetargeted]("mining.retargeted")
	register[MiningStarted]("mining.started")
	register[MiningStopped]("mining.stopped")

	register[ReplicantEntered]("multiplayer.replicant_entered")
	register[ReplicantLeft]("multiplayer.replicant_left")

	register[PrintCancelled]("print.cancelled")
	register[PrintCompleted]("print.completed")
	register[PrintStarted]("print.started")

	register[ProspectCompleted]("prospect.completed")

	register[RelayActivated]("relay.activated")

	register[ReplicantTransferred]("replicant.transferred")

	register[SalvageDepleted]("salvage.depleted")
	register[SalvageDiscovered]("salvage.discovered")

	register[ScanCompleted]("scan.completed")
	register[ScanStarted]("scan.started")

	register[SearchCompleted]("search.completed")
	register[SearchStarted]("search.started")

	register[SimulationAbandoned]("simulation.abandoned")
	register[SimulationCompleted]("simulation.completed")
	register[SimulationExpired]("simulation.expired")
	register[SimulationStarted]("simulation.started")

	register[SiteDepleted]("site.depleted")

	register[NewReplicant]("story.awakened")
	register[StoryHint]("story.hint")

	register[SystemBodyRenamed]("system.body_renamed")
	register[SystemDevicesHalted]("system.devices_halted")
	register[SystemEntryPointSet]("system.entry_point_set")
	register[SystemImpactWarning]("system.impact_warning")
	register[SystemObjectDetected]("system.object_detected")
	register[SystemObjectDiverted]("system.object_diverted")
	register[SystemObjectImpacted]("system.object_impacted")
	register[SystemObjectOffTarget]("system.object_off_target")

	register[TeleportCompleted]("teleport.completed")
	register[TeleportFailed]("teleport.failed")
	register[TeleportStarted]("teleport.started")

	register[TerraformActivated]("terraform.activated")
	register[TerraformDeactivated]("terraform.deactivated")
	register[TerraformDeviceActivated]("terraform.device_activated")
	register[TerraformDeviceDeactivated]("terraform.device_deactivated")
	register[TerraformStatus]("terraform.status")

	register[TradeCompleted]("trade.completed")
	register[TradeCreated]("trade.created")
	register[TradeDeleted]("trade.deleted")

	register[TransportCollected]("transport.collected")
	register[TransportDelivered]("transport.delivered")

	register[TravelArrived]("travel.arrived")
	register[TravelCancelled]("travel.cancelled")
	register[TravelDeparted]("travel.departed")

	register[TriangulationComplete]("triangulation.complete")
	register[TriangulationFailed]("triangulation.failed")
	register[TriangulationStarted]("triangulation.started")

	register[WardActivated]("ward.activated")
	register[WardDeactivated]("ward.deactivated")
}

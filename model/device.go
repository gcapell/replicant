package model

type ResourceType string

const (
	Carbon     ResourceType = "carbon"
	Conductive ResourceType = "conductive"
	Rares      ResourceType = "rares"
	Silicates  ResourceType = "silicates"
	Structural ResourceType = "structural"
	Volatiles  ResourceType = "volatiles"
)

type DeviceList struct {
	Devices []Device
	Paged
}

type Device struct {
	AvailableCommands    []string
	DeviceCode           string
	DeviceType           string
	Features             []string
	HostingReplicant     ReplicantPtr
	InControlRange       bool
	Location             string
	OperationalCapacity  float64
	ControllerDeviceCode string
	StowedInDeviceCode   string
	// print_queue
	ReplicantCode        string
	Status               string
	StowCapacity         int
	StowUsed             int
	StowedDevices        []StowedDevice
	Travel               Travel
	AttachedToDeviceCode string
	AttachedDevices      []AttachedDevice

	Cargo         []ResourceCount
	CargoCapacity int
	CargoUsed     int

	AmiDirectiveStatus string
	Amidirective       AMIDirective
	Printing           PrintingStatus

	WaitingFor map[ResourceType]WaitingForStat

	PrintQueue []DevicePrintQueueItem
	Prospect   PendingStatus
	Compact    PendingStatus
	Scan       ScanStatus
}

type ReplicantPtr struct {
	name          string
	ReplicantCode string
}

type StowedDevice struct {
	DeviceCode string
	DeviceType string
	Status     string
}

type Travel struct {
	FinalArrivesAt   string
	Origin           string
	DepartedAt       string
	FinalDestination string
	RouteEtaSeconds  float64
	EtaSeconds       int
	Stage            string
	Type             string
}

type AttachedDevice struct {
	DeviceCode string
	DeviceType string
}

type ResourceCount struct {
	ResourceType string
	Quantity     int
}

type AMIDirective struct {
	Name   string
	Config AMIDirectiveConfig
}

type AMIDirectiveConfig struct {
}

type PendingStatus struct {
	CompletesAt     string
	EtaSeconds      int
	ProgressPercent float64
	StartedAt       string
}

type PrintingStatus struct {
	PendingStatus
	DeviceType string
	// tags
}

type WaitingForStat struct {
	Have int
	Need int
}

type ScanStatus struct {
	PendingStatus
	Target string
}
type DevicePrintQueueItem struct {
	DeviceType string
}

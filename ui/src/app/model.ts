
export interface AccountData {
    experience_points_total: number;
    replicants: ReplicantSummary[];
    unread_message_count: number;
}

export interface ReplicantSummary {
    created_at: string;
    current_location: string;
    current_star: string;
    device_count: number;
    experience_points: number;
    hosted_device_code: string;
    name: string;
    replicant_code: string;
}

export interface Replicant {
    attached_devices: Device[];
    cargo: CargoItem[];
    experience_points: number;
    hosted_device_code: string;
    location: string;
    name: string;
    position: Position;
    print_queue: PrintQueueItem[];
    printing: CurrentlyPrinting;
    replicant_code: string;
    status: string;
    stowed_devices: StowedDevice[];
}

interface CurrentlyPrinting {
    device_type:string;
    completes_at:string;
    eta_seconds:number;
    progress_percent:number;
}

interface StowedDevice {
    device_code: string;
    device_type: string;
    status: string;
}

interface CargoItem {
    item_code: string;
    quantity: number;
}

interface Position {
    x: number;
    y: number;
    z: number;
}

interface PrintQueueItem {
    item_code: string;
    quantity: number;
}

export interface Location {
    location: string;
    devices: Device[];
    name: string;
    position: Position;
    inventory: ResourceCount[];
    resource_sites: ResourceSite[];
}

interface ResourceSite{
    designation: string;
    name:string;
    resources_remaining_pct: Record<string,number>;
}
interface ResourceCount {
    resource_type: string;
    quantity: number;
}

export interface SearchResult {

}

export interface Tutorials {
    tutorials: TutorialSummary[];
}

export interface TutorialSummary {
    slug: string;
    name: string;
    description: string;
    order: number;
    completed: boolean;
    current_step: number;
    total_steps: number;
}

export interface TutorialDetail {
    completed: boolean;
    current_step: number;
    description: string;
    name: string,
    slug: string,
    steps: TutorialStep[]
}

interface TutorialStep {
    completed: boolean;
    current: boolean;
    description: string;
    hint: string;
    key: string;
}

export interface MessageList {
    messages: Message[];
}
interface Message {
    body: string;
    category: string;
    created_at: string;
    id: number;
    is_read: boolean;
    message_type: string,
    subcategory: string,
    title: string,
}

export interface StartMiningResult {
    cycle_time_seconds: number;
    device_code: string;
    location:string;
    status:string;
}

export interface ReplicantScanResult{
    active_location_events: ActiveLocationEvent[];
    asteroid_belt: AsteroidBelts;
    entry_point:string;
    life_detected: boolean;
    mining_bonus_pct: number;
    outer_system: Record<string,OuterSystem>;
    planets: Planet[];
    replicants: ReplicantFoundByScan[]
    shops: Shop[];
    system_objects: SystemObject[];
    system_tags: string[];
}

interface SystemObject{
    designation:string;
    impact_eta: string;
    impact_likelihood: number;
    impact_target: string;
    object_type: string;
    status:string;
}
interface Shop{}

interface ReplicantFoundByScan{
    last_active:string;
    location:string;
    name:string;
    replicant_code:string;
}

interface Planet {
    designation: string;
    in_habitable_zone: boolean;
    moon_count: number;
    scanned: boolean;
    salvage: Salvage[];
    type: string;
}

interface Salvage{
    depleted: boolean;
    designation: string;
    location:string;
    name:string;
    resources_available: string[];
    salvage_type: string;
}

interface OuterSystem{
    designation: string;
    distance_au: number;
}

interface AsteroidBelts {
    belts: AsteroidBelt[];
}

interface AsteroidBelt{
    resources: Record<string,string>;
    designation:string;
}
interface ActiveLocationEvent{}

export interface BlueprintsList{
    blueprints: Blueprint[];
}

interface Blueprint{
    attach_capacity: number;
    cargo_capacity:number;
    // components?
    description:string;
    device_type: string;
    directives: string[];
    features: string[];
    print_time: number;
    queue_size: number;
    relay_range: number;
    resources: Record<string,number>;
    short_description:string;
    stow_capacity:number;
    strength:number;
}

export interface DeviceList {
    devices: Device[]
    next_cursor: number;
}

interface Device {
    available_commands: string[];
    device_code:string;
    device_type:string;
    features: string[];
    hosting_replicant: ReplicantPtr;
    in_control_range: boolean;
    location:string;
    operational_capacity: number;
    // print_queue
    replicant_code: string;
    status: string;
    stow_capacity: number;
    stow_used: number;
    stowed_devices: StowedDevice[];
    travel: Travel;

    cargo: ResourceCount[];
    cargo_capacity:number;
    cargo_used:number;
}

interface Travel {
    final_arrives_at: string;
    origin:string;
    departed_at: string;
    final_destination:string;
    route_eta_seconds: string;
    stage: string;
    type: string;
}

interface ReplicantPtr{
    name:string;
    replicant_code: string;
}

export interface SendDroneResult{

}

export interface DeactivateDroneResult{

}

export interface ScanDroneResult{
    
}

export interface StartPrintingResult {

}

export interface FillTransportResult{

}
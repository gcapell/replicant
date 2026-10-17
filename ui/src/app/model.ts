
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
    replicant_code: string;
    status: string;
    stowed_devices: Device[];
}

interface Device {
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
export interface EventList {
    events: GameEvent[];
}
interface EventBase {
    category: string;
    created_at: string;
    device_code: string | null;
    device_type: string | null;
    event: string;
    id: string;
    location: string | null;
    star: string | null;
    version: number;
}
interface BlueprintUnlockedEvent extends EventBase {
    event: 'blueprint.unlocked';
    payload: {
        device_type: string;
        description: string;
        short_description: string;
        print_time: number;
        requires_autofactory: boolean;
        resources: Record<string, number>;
    }
}
interface MessageNewEvent extends EventBase {
    event: 'message.new';
    payload: {
        message_id: number;
        message_type: string;
        title: string;
        body: string;
    };
}

interface MiningRetargetedEvent extends EventBase{
    event: 'mining.retargeted';
    payload: {
        availability: string;
        belt: string;
        cycle_time_seconds: number;
        density: string;
        designation: string;
        location: string;
        location_type: string;
        new_resource: string;
        old_resource: string;
        salvage_type: string | null;
        site: string;
    }
}
interface PrintStartedEvent extends EventBase{
    event: 'print.started';
    payload: {
        completes_at: string;
        device_type: string;
        print_mode: string;
    }
}
interface MiningResourceDepletedEvent extends EventBase {
    event: 'mining.resource_depleted';
    payload: {
        location: string;
        quantity_mined: number;
        resource: string;
        resource_type: string;
        site: string;
    }
}

interface MiningStoppedEvent extends EventBase{
    event: 'mining.stopped';
    payload: {
        belt: string;
        location: string;
        quantity_mined: number;
        resource_type: string;
    }
}

interface ExperienceGainedEvent extends EventBase{
    event: 'experience.gained';
    payload: {
        amount: number;
        source:string;
    }
}

interface PrintCompletedEvent extends EventBase{
    event: 'print.completed';
    payload: {
        compacted: boolean;
        device_type: string;
        new_device_code: string;
        print_mode: string;
    }
}

interface MiningStartedEvent extends EventBase{
    event: 'mining.started';
    payload: {
        cycle_time_seconds: number;
        resource_type: string;
        site:string;
        // incomplete
    }
}

interface SiteDepletedEvent extends EventBase{
    event: 'site.depleted';
    payload: {
        site:string;
    }
}

interface DeviceStowedEvent extends EventBase{
    event: 'device.stowed';
    payload: {
        stowed_in_device_code:string;
    }
}
interface DeviceDeployedEvent extends EventBase{
    event: 'device.deployed';
    payload: {
        deployed_from:string;
    }
}

interface ScanStartedEvent extends EventBase{
    event: 'scan.started';
    payload: {
        completes_at:string;
        eta_seconds:number;
        scan_target:string;
        scan_type: string;
    }
}

interface ScanCompletedEvent extends EventBase{
      event: 'scan.completed';
    payload: {
        report: Record<string,unknown>;
        scan_target:string;
        scan_type: string;
    }  
}

interface SearchStartedEvent extends EventBase{
    event: 'search.started';
    payload: {
        completes_at: string;
        eta_seconds: number;
        search_target:string;
        search_type: string;
    }
}

interface SearchCompletedEvent extends EventBase{
    event: 'search.completed';
    payload: {
        report: SearchReport;
        search_target:string;
        search_type: string;
    }
}

interface SearchReport {
    site:string;
    resources: Record<string,number>;
}

type GameEvent = BlueprintUnlockedEvent |
    MessageNewEvent |
    MiningRetargetedEvent |
    PrintStartedEvent |
    MiningResourceDepletedEvent |
    MiningStoppedEvent |
    ExperienceGainedEvent |
    PrintCompletedEvent |
    SiteDepletedEvent |
    DeviceStowedEvent |
    DeviceDeployedEvent |
    ScanStartedEvent |
    ScanCompletedEvent |
    SearchStartedEvent |
    SearchCompletedEvent |
    MiningStartedEvent;
export interface EventList {
    events: GameEvent[];
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

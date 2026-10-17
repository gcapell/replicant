
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
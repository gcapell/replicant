
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
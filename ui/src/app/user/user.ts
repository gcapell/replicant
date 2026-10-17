import { Component, signal, inject, WritableSignal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { JsonPipe } from '@angular/common';

interface AccountData {
  experience_points_total: number;
  replicants: ReplicantSummary[];
  unread_message_count: number;
}

interface ReplicantSummary {
  created_at: string;
  current_location: string;
  current_star: string;
  device_count: number;
  experience_points: number;
  hosted_device_code: string;
  name: string;
  replicant_code: string;
}

interface Replicant {
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

@Component({
  imports: [JsonPipe],
  selector: 'app-user',
  styleUrl: './user.scss',
  templateUrl: './user.html',
})
export class User {
  protected readonly http = inject(HttpClient);
  protected readonly accountData = signal<AccountData|null>(null);
  protected readonly replicant = signal<Replicant|null>(null);
  protected readonly errorMessage = signal<unknown>(null);
  protected readonly token = signal(localStorage.getItem('token') ?? '');

  private fetchAndUpdate<T>(path: string, target: WritableSignal<T | null>) {
  this.http.get<T>(path, {
      headers: {
        'Authorization': `Bearer ${this.token()}`
      }
    }).subscribe({
      next: (data) => {
        target.set(data);
      },
      error: (error) => {
        this.errorMessage.set(error);
        console.error(`Error loading ${path}:`, error);
      }
    });
}
  protected loadReplicant(id:string) {
    this.fetchAndUpdate(`/v1/replicants/${id}`, this.replicant);
  }
  protected loadAccount(token: string) {
    localStorage.setItem('token', token);
    this.token.set(token);
    this.fetchAndUpdate(`/v1/accounts/me`, this.accountData);
  }
}

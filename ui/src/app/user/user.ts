import { Component, signal, inject } from '@angular/core';
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

  protected loadReplicant(id:string) {
    this.http.get<Replicant>(`/v1/replicants/${id}`, {
      headers: {
        'Authorization': `Bearer ${this.token()}`
      }
    }).subscribe({
      next: (data) => {
        this.replicant.set(data);
      },
      error: (error) => {
        this.errorMessage.set(error);
        console.error('Error loading replicant data:', error);
      }
    });
  }
  protected loadUser(token: string) {
    this.http.get<AccountData>(`/v1/accounts/me`, {
      headers: {
        'Authorization': `Bearer ${token}`
      }
    }).subscribe({
      next: (accountData) => {
        localStorage.setItem('token', token);
        this.accountData.set(accountData);
        console.log('User loaded:', accountData);
      },
      error: (error) => {
        this.errorMessage.set(error);
        console.error('Error loading account data:', error);
      }
    });
  }
}

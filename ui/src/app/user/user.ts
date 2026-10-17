import { Component, signal, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { JsonPipe } from '@angular/common';

interface AccountData {
  experience_points_total: number;
  replicants: Replicant[];
  unread_message_count: number;
}

interface Replicant {
  created_at: string;
  current_location: string;
  current_star: string;
  device_count: number;
  experience_points: number;
  hosted_device_code: string;
  name: string;
  replicant_code: string;
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
  protected readonly errorMessage = signal<unknown>(null);
  protected readonly token = signal(localStorage.getItem('token') ?? '');

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

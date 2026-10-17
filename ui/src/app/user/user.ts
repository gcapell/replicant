import { Component, signal, inject, WritableSignal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { JsonPipe } from '@angular/common';
import { AccountData, Replicant } from '../model';

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
        target.set(null);
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

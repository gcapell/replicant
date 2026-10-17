import { Component, signal, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';

@Component({
  imports: [],
  selector: 'app-user',
  styleUrl: './user.scss',
  templateUrl: './user.html',
})

export class User {
    protected readonly http = inject(HttpClient);

  protected loadUser(token: string) {
    this.http.get(`/v1/accounts/me`, {
      headers: {
        'Authorization': `Bearer ${token}`
      }
    }).subscribe({
      next: (user) => {
        console.log('User loaded:', user);
      },
      error: (error) => {
        console.error('Error loading user:', error);
      }
    });
  }
}

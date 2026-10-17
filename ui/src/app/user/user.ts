import { Component, inject } from '@angular/core';
import { JsonPipe } from '@angular/common';
import { ReplicantApi } from '../replicant-api';

@Component({
  imports: [JsonPipe],
  selector: 'app-user',
  styleUrl: './user.scss',
  templateUrl: './user.html',
})
export class User {
  protected readonly api = inject(ReplicantApi);
}

import { Component, inject } from '@angular/core';
import { ReplicantApi } from '../replicant-api';
import { JsonPipe, KeyValuePipe } from '@angular/common';

@Component({
  imports: [JsonPipe, KeyValuePipe],
  selector: 'app-replicant',
  styleUrl: './replicant.scss',
  templateUrl: './replicant.html',
})
export class Replicant {
  protected readonly api = inject(ReplicantApi);
}

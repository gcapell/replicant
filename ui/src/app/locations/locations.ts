import { Component, inject } from '@angular/core';
import { JsonPipe, KeyValuePipe } from '@angular/common';
import { ReplicantApi } from '../replicant-api';

@Component({
  imports: [JsonPipe, KeyValuePipe],
  selector: 'app-locations',
  styleUrl: './locations.scss',
  templateUrl: './locations.html',
})
export class Locations {
  protected readonly api = inject(ReplicantApi);
}

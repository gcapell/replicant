import { Component, inject } from '@angular/core';
import { JsonPipe } from '@angular/common';
import { ReplicantApi } from '../replicant-api';

@Component({
  imports: [JsonPipe],
  selector: 'app-locations',
  styleUrl: './locations.scss',
  templateUrl: './locations.html',
})
export class Locations {
  protected readonly api = inject(ReplicantApi);
}

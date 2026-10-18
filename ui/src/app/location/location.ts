import { Component, inject } from '@angular/core';
import { ReplicantApi } from '../replicant-api';
import { JsonPipe } from '@angular/common';

@Component({
  imports: [JsonPipe],
  selector: 'app-location',
  styleUrl: './location.scss',
  templateUrl: './location.html',
})
export class Location {
  protected readonly api = inject(ReplicantApi);
}

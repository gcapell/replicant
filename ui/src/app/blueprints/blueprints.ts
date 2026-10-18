import { Component, inject } from '@angular/core';
import { ReplicantApi } from '../replicant-api';
import { JsonPipe } from '@angular/common';

@Component({
  imports: [JsonPipe],
  selector: 'app-blueprints',
  styleUrl: './blueprints.scss',
  templateUrl: './blueprints.html',
})
export class Blueprints {
  protected readonly api = inject(ReplicantApi);
}

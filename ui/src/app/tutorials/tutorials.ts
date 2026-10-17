import { Component, inject } from '@angular/core';
import { ReplicantApi } from '../replicant-api';
import { JsonPipe } from '@angular/common';

@Component({
  imports: [JsonPipe],
  selector: 'app-tutorials',
  styleUrl: './tutorials.scss',
  templateUrl: './tutorials.html',
})
export class Tutorials {
  protected readonly api = inject(ReplicantApi);
}

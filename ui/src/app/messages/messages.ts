import { Component, inject } from '@angular/core';
import { ReplicantApi } from '../replicant-api';
import { JsonPipe } from '@angular/common';

@Component({
  imports: [JsonPipe],
  selector: 'app-messages',
  styleUrl: './messages.scss',
  templateUrl: './messages.html',
})
export class Messages {
protected readonly api = inject(ReplicantApi);
}

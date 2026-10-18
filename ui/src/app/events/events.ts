import { Component, inject } from '@angular/core';
import { ReplicantApi } from '../replicant-api';
import { JsonPipe } from '@angular/common';
import { KvTable } from '../shared/kv-table/kv-table';

@Component({
  imports: [JsonPipe, KvTable],
  selector: 'app-events',
  styleUrl: './events.scss',
  templateUrl: './events.html',
})
export class Events {
  protected readonly api = inject(ReplicantApi);
}

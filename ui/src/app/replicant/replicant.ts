import { Component, inject } from '@angular/core';
import { ReplicantApi } from '../replicant-api';
import { JsonPipe } from '@angular/common';
import { KvTable } from '../shared/kv-table/kv-table';

@Component({
  imports: [JsonPipe, KvTable],
  selector: 'app-replicant',
  styleUrl: './replicant.scss',
  templateUrl: './replicant.html',
})
export class Replicant {
  protected readonly api = inject(ReplicantApi);
}

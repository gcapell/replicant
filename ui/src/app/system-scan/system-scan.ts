import { Component, inject } from '@angular/core';
import { ReplicantApi } from '../replicant-api';
import { JsonPipe, KeyValuePipe } from '@angular/common';
import { KvTable } from '../shared/kv-table/kv-table';

@Component({
  imports: [JsonPipe, KvTable, KeyValuePipe],
  selector: 'app-system-scan',
  styleUrl: './system-scan.scss',
  templateUrl: './system-scan.html',
})
export class SystemScan {
  protected readonly api = inject(ReplicantApi);
}

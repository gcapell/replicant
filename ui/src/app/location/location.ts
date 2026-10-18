import { Component, inject } from '@angular/core';
import { ReplicantApi } from '../replicant-api';
import { JsonPipe } from '@angular/common';
import {KvTable} from '../shared/kv-table/kv-table';

@Component({
  imports: [JsonPipe, KvTable],
  selector: 'app-location',
  styleUrl: './location.scss',
  templateUrl: './location.html',
})
export class Location {
  protected readonly api = inject(ReplicantApi);
}

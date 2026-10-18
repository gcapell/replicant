import { Component, input} from '@angular/core';
import { KeyValuePipe } from '@angular/common';

@Component({
  imports: [KeyValuePipe],
  selector: 'app-kv-table',
  styleUrl: './kv-table.scss',
  templateUrl: './kv-table.html',
})
export class KvTable {
  items = input<Record<string,unknown> | null>();
}

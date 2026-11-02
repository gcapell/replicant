import { Component, inject, signal, computed } from '@angular/core';
import { JsonPipe, KeyValuePipe } from '@angular/common';
import { ReplicantApi, SystemSummary } from '../replicant-api';
import { LocationSummary } from '../model';



interface Row {
  name: string;
  summary: LocationSummary;
  isSystem: boolean;
  systemKey: string;
  expanded?: boolean;
}

@Component({
  imports: [JsonPipe, KeyValuePipe],
  selector: 'app-locations',
  styleUrl: './locations.scss',
  templateUrl: './locations.html',
})
export class Locations {
  protected readonly api = inject(ReplicantApi);
  expanded_systems = signal<Set<string>>(new Set());

  rows = computed<Row[]>(() => {
    const result: Row[] = [];
    const systems = this.api.systems();
    const expanded = this.expanded_systems();

    for (const [key, sys] of systems) {
      const isOpen = expanded.has(key);
      result.push({
        name: key,
        summary: sys.summary,
        isSystem: true,
        systemKey: key,
        expanded: isOpen,
      });

      if (isOpen) {
        for (const [loc, summary] of sys.children) {
          result.push({
            name: loc,
            summary: summary,
            isSystem: false,
            systemKey: key,
          });
        }
      } else {
        console.log("key", key, "not in", expanded);
      }
    }
    return result;
  });

  toggle(systemKey: string) {
    console.log("toggling", systemKey);
    this.expanded_systems.update(set => {
      console.log("before", set)
      const next = new Set(set); // new Set so the signal sees a change
      next.has(systemKey) ? next.delete(systemKey) : next.add(systemKey);
      console.log("after", next);
      return next;
    });
  }


}

import { Component, inject, signal } from '@angular/core';
import { DecimalPipe } from '@angular/common';
import { ReplicantApi, systemOf } from '../replicant-api';
import { LocationInventory } from '../model';

export interface ResourceTable {
  columns: string[]; // resource types, sorted
  rows: { star: string; amounts: Record<string, number> }[]; // sorted by star
}

function itemsOf(li: LocationInventory): [string, number][] {
  return Array.isArray(li.items)
    ? li.items.map((i) => [i.resource_type, i.quantity])
    : Object.entries(li.items);
}

// tabulate sums each star's stockpiles by resource type.
export function tabulate(inv: LocationInventory[]): ResourceTable {
  const columns = new Set<string>();
  const byStar = new Map<string, Record<string, number>>();
  for (const li of inv) {
    const star = systemOf(li.location);
    let amounts = byStar.get(star);
    if (!amounts) {
      amounts = {};
      byStar.set(star, amounts);
    }
    for (const [type, q] of itemsOf(li)) {
      columns.add(type);
      amounts[type] = (amounts[type] ?? 0) + q;
    }
  }
  return {
    columns: [...columns].sort(),
    rows: [...byStar.keys()].sort().map((star) => ({ star, amounts: byStar.get(star)! })),
  };
}

@Component({
  imports: [DecimalPipe],
  selector: 'app-resources',
  styleUrl: './resources.scss',
  templateUrl: './resources.html',
})
export class Resources {
  protected readonly api = inject(ReplicantApi);
  readonly table = signal<ResourceTable | null>(null);
  readonly loading = signal(false);

  async fetch() {
    this.loading.set(true);
    try {
      this.table.set(tabulate(await this.api.inventory()));
    } catch (e) {
      this.api.errorMessage.set(e);
      console.error('Error loading /v1/inventory:', e);
    } finally {
      this.loading.set(false);
    }
  }
}

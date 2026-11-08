import { Component, inject, signal } from '@angular/core';
import { ReplicantApi } from '../replicant-api';
import { LocationEvent, LocationEventOption, LocationEventRewards, ResourceType } from '../model';

const ABBREV: Record<ResourceType, string> = {
  carbon: 'C',
  conductive: 'Co',
  rares: 'R',
  silicates: 'Si',
  structural: 'S',
  volatiles: 'V',
};
const RESOURCE_ORDER = Object.keys(ABBREV) as ResourceType[];

const abbrev = (r: string): string => ABBREV[r as ResourceType] ?? r;

// Everything the option asks for is at the location. The API's own `met`
// also needs a replicant there, so we don't use it.
export function optionSatisfied(o: LocationEventOption): boolean {
  return [...(o.resources ?? []), ...(o.devices ?? [])].every((x) => x.current >= x.required);
}

// One option: C(0/100), defence_grid(0/1), with a tick when it's satisfied.
export function formatOption(o: LocationEventOption): string {
  const items = [
    ...(o.resources ?? []).map((r) => `${abbrev(r.resource_type)}(${r.current}/${r.required})`),
    ...(o.devices ?? []).map((d) => `${d.device_type}(${d.current}/${d.required})`),
  ];
  return `${items.join(', ')}${optionSatisfied(o) ? ' ✅' : ''}`;
}

// One string per alternative.
export function formatRequest(e: LocationEvent): string[] {
  return (e.progress?.options ?? []).map(formatOption);
}

// Resource rewards only, in a fixed order: C500, R50.
export function formatReward(r: LocationEventRewards | undefined): string {
  const res = r?.resources ?? {};
  const items = RESOURCE_ORDER.filter((k) => res[k]).map((k) => `${ABBREV[k]}${res[k]}`);
  return items.join(', ');
}

@Component({
  selector: 'app-location-events',
  styleUrl: './location-events.scss',
  templateUrl: './location-events.html',
})
export class LocationEvents {
  protected readonly api = inject(ReplicantApi);
  // Active events from /v1/accounts/events, sorted by location; null until fetched.
  readonly events = signal<LocationEvent[] | null>(null);
  readonly loading = signal(false);

  protected readonly formatRequest = formatRequest;
  protected readonly formatReward = formatReward;

  async fetch() {
    this.loading.set(true);
    try {
      const events = await this.api.accountEvents();
      this.events.set(events.sort((a, b) => a.location.localeCompare(b.location)));
    } catch (e) {
      this.api.errorMessage.set(e);
      console.error('Error loading location events:', e);
    } finally {
      this.loading.set(false);
    }
  }
}

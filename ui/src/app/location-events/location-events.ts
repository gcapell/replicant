import { Component, computed, inject, signal } from '@angular/core';
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

// One option: C(0/100), defence_grid(0/1), with a tick when it's met.
export function formatOption(o: LocationEventOption): string {
  const items = [
    ...(o.resources ?? []).map((r) => `${abbrev(r.resource_type)}(${r.current}/${r.required})`),
    ...(o.devices ?? []).map((d) => `${d.device_type}(${d.current}/${d.required})`),
  ];
  return `${items.join(', ')}${o.met ? ' ✅' : ''}`;
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
  readonly events = signal<Map<string, LocationEvent>>(new Map());
  readonly loading = signal(false);

  // Locations with an event, from the last /v1/locations fetch, sorted by code.
  readonly locations = computed<string[]>(() =>
    Object.entries(this.api.locationList()?.locations ?? {})
      .filter(([, s]) => s.location_events > 0)
      .map(([loc]) => loc)
      .sort(),
  );

  protected readonly formatRequest = formatRequest;
  protected readonly formatReward = formatReward;

  async fetch() {
    this.loading.set(true);
    try {
      if (!this.api.locationList()) await this.api.locationListOnly();
      const fetched = new Map<string, LocationEvent>();
      await Promise.all(
        this.locations().map(async (loc) => {
          const l = await this.api.locationDetails(loc);
          if (l.location_event) fetched.set(loc, l.location_event);
        }),
      );
      this.events.set(fetched);
    } catch (e) {
      this.api.errorMessage.set(e);
      console.error('Error loading location events:', e);
    } finally {
      this.loading.set(false);
    }
  }
}

import { Component, input, output, WritableSignal } from '@angular/core';

export interface OrphanDrone {
  device_code: string;
  device_type: string;
  location: string;
}

@Component({
  imports: [],
  selector: 'app-adopt',
  styleUrl: './adopt.scss',
  templateUrl: './adopt.html',
})
export class Adopt {
  orphans = input.required<string[]>();
  device_by_id = input.required<Map<string,WritableSignal<OrphanDrone>>>();
  adopt = output<string[]>();

  selected(select: HTMLSelectElement): string[] {
    return Array.from(select.selectedOptions).map(o => o.value);
  }
}

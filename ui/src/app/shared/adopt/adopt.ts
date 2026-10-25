import { Component, input, output } from '@angular/core';

export interface OrphanDrone {
  device_code: string;
  location: string;
}

@Component({
  imports: [],
  selector: 'app-adopt',
  styleUrl: './adopt.scss',
  templateUrl: './adopt.html',
})
export class Adopt {
  orphans = input.required<OrphanDrone[]>();
  adopt = output<string[]>();

  selected(select: HTMLSelectElement): string[] {
    return Array.from(select.selectedOptions).map(o => o.value);
  }
}

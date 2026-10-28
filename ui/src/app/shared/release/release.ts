import { Component, input, output } from '@angular/core';

export interface ChildDrone {
  device_code: string;
  location: string;
}

@Component({
  imports: [],
  selector: 'app-release',
  styleUrl: './release.scss',
  templateUrl: './release.html',
})


export class Release {
  children = input.required<string[]>();
  device_by_id = input.required<Record<string,ChildDrone>>();
  release = output<string[]>();

  selected(select: HTMLSelectElement): string[] {
    return Array.from(select.selectedOptions).map(o => o.value);
  }
}

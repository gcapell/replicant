import { Component, computed, inject, input, signal } from '@angular/core';
import { ReplicantApi } from '../../replicant-api';

let nextId = 0;

@Component({
  imports: [],
  selector: 'app-move-device',
  styleUrl: './move-device.scss',
  templateUrl: './move-device.html',
})
export class MoveDevice {
    protected readonly api = inject(ReplicantApi);
    deviceCode = input.required<string>();
    destinations = input.required<string[]>();

    // Each instance needs its own datalist id, since many can be on one page.
    protected readonly listId = `move-device-destinations-${nextId++}`;
    protected readonly value = signal('');
    protected readonly valid = computed(() => (this.destinations() ?? []).includes(this.value()));
}

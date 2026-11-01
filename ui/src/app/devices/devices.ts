import { Component, inject, computed, signal } from '@angular/core';
import { ReactiveFormsModule } from '@angular/forms';
import { ReplicantApi } from '../replicant-api';
import { JsonPipe, KeyValuePipe } from '@angular/common';
import { DeviceComponent } from '../device/device';

const systemOf = (loc: string): string => loc?.split("-", 1)[0];

@Component({
  imports: [ReactiveFormsModule, KeyValuePipe, DeviceComponent, JsonPipe],
  selector: 'app-devices',
  styleUrl: './devices.scss',
  templateUrl: './devices.html',
})
export class Devices {
  protected readonly api = inject(ReplicantApi);

  selected_system = signal<string>("");

  async select_system(system: string) {
    this.selected_system.set(system);
    await this.api.listDevicesAsync(system);
  }
}

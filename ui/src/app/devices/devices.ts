import { Component, inject, computed, Pipe, PipeTransform } from '@angular/core';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { ReplicantApi } from '../replicant-api';
import { JsonPipe, KeyValuePipe } from '@angular/common';
import { MoveDevice } from '../shared/move-device/move-device';
import { Device } from '../model';
import { humanizeSeconds } from '../format-duration';

@Pipe({ name: 'humanizeSeconds', standalone: true })
export class HumanizeSecondsPipe implements PipeTransform {
  transform(value: number): string {
    return humanizeSeconds(value);
  }
}

interface DeviceTree {
  devices: Device[];
  controlled: Record<string, Device[]>; // devices controlled by this ID
  stowed: Record<string, Device[]>;  // devices stowed in this ID
}

@Component({
  imports: [JsonPipe, MoveDevice, ReactiveFormsModule, HumanizeSecondsPipe, KeyValuePipe],
  selector: 'app-devices',
  styleUrl: './devices.scss',
  templateUrl: './devices.html',
})
export class Devices {
  protected readonly api = inject(ReplicantApi);
  private fb = inject(FormBuilder);

  salvageSites(location: string): string[] {
    const planet = this.api.replicantScanResult()?.planets.find(
      p => p.designation === location
    );
    return planet?.salvage
      ?.filter(s => !s.depleted)
      .map(s => s.designation) ?? [];
  }
  adopt(controller: string, select: HTMLSelectElement) {
    const codes = Array.from(select.selectedOptions, o => o.value);
    this.api.adoptTransport(controller, codes);
  }


  deviceTree = computed<DeviceTree | null>(() => {
    const deviceList = this.api.deviceList()?.devices;
    if (!deviceList?.length) return null;

    const controlled: Record<string, Device[]> = {};
    const stowed: Record<string, Device[]> = {};
    const devices: Device[] = [];
    for (const d of deviceList) {
      if (d.controller_device_code) {
        (controlled[d.controller_device_code] ??= []).push(d);
      } else if (d.stowed_in_device_code) {
        (stowed[d.stowed_in_device_code] ??= []).push(d);
      } else {
        devices.push(d);
      }
    }
    return { devices, controlled, stowed };
  });

  form = this.fb.nonNullable.group({
    collect: ['', Validators.required],
    deliver: ['', Validators.required],
    items: this.fb.array([this.newItem()], Validators.minLength(1)),
  });
  newItem() {
    return this.fb.nonNullable.group({
      resource: ['', Validators.required],
      amount: [0, [Validators.required, Validators.min(1)]],
    });
  }
  addItem() {
    this.form.controls.items.push(this.newItem());
  }
  removeItem(i: number) {
    this.form.controls.items.removeAt(i);
  }

  setDeliveryDirective(controller: string) {
    if (this.form.invalid) return;
    const v = this.form.getRawValue();
    const requirement = Object.fromEntries(v.items.map(i => [i.resource, i.amount]));
    this.api.setDeliveryDirective(controller, v.collect, v.deliver, requirement);
  }
}

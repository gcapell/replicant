import { Component, inject , computed } from '@angular/core';
import { ReplicantApi } from '../replicant-api';
import { JsonPipe } from '@angular/common';
import { MoveDevice } from '../shared/move-device/move-device';
import {Device } from '../model';


interface DeviceTree {
  devices: Device[];
  controlled: Record<string, Device[]>; // devices controlled by this ID
  stowed: Record<string, Device[]>;  // devices stowed in this ID
}

@Component({
  imports: [JsonPipe, MoveDevice],
  selector: 'app-devices',
  styleUrl: './devices.scss',
  templateUrl: './devices.html',
})
export class Devices {
  protected readonly api = inject(ReplicantApi);

  salvageSites(location: string): string[] {
    const planet = this.api.replicantScanResult()?.planets.find(
      p => p.designation === location
    );
    return planet?.salvage
      ?.filter(s => !s.depleted)
      .map(s => s.designation) ?? [];
  }
  adopt(controller:string, select: HTMLSelectElement){
    const codes = Array.from(select.selectedOptions, o => o.value);
    this.api.adoptTransport(controller, codes);
  }


  deviceTree = computed<DeviceTree|null>(()=>{
    const deviceList = this.api.deviceList()?.devices;
    if (!deviceList?.length) return null;

    const controlled: Record<string, Device[]> = {};
    const stowed: Record<string, Device[]> = {};
    const devices: Device[] = [];
    for (const d of deviceList){
      if (d.controller_device_code) {
        (controlled[d.controller_device_code] ??=[]).push(d);
      } else if (d.stowed_in_device_code) {
        (stowed[d.stowed_in_device_code] ??=[]).push(d);
      } else {
        devices.push(d);
      }
    }
    return {devices, controlled, stowed};
  });
}

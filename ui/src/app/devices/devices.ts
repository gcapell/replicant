import { Component, inject, computed, signal } from '@angular/core';
import { ReactiveFormsModule } from '@angular/forms';
import { ReplicantApi } from '../replicant-api';
import { JsonPipe, KeyValuePipe } from '@angular/common';
import { Device } from '../model';
import {DeviceComponent,DeviceTree} from '../device/device';


const systemOf = (loc: string): string  => loc?.split("-", 1)[0];

@Component({
  imports: [ ReactiveFormsModule, KeyValuePipe,DeviceComponent,JsonPipe],
  selector: 'app-devices',
  styleUrl: './devices.scss',
  templateUrl: './devices.html',
})
export class Devices {
  protected readonly api = inject(ReplicantApi);

  system_visible = signal<Set<string>>(new Set<string>());
  deviceTree = computed<DeviceTree | null>(() => {
    const deviceList = this.api.deviceList()?.devices;
    if (!deviceList?.length) return null;

    const controlled: Record<string, string[]> = {};
    const stowed: Record<string, string[]> = {};
    const devices: Device[] = [];
    const orphan_mining_drones: string[] = [];
    const orphan_survey_drones: string[] = [];
    const orphan_transport_drones: string[] = [];
    const by_system: Record<string, string[]> = {};
    const by_id: Record<string, Device> = {};

    // id -> other IDs at same location (for attachment etc)
    const location_neighbors: Record<string, string[]> = {}; 

    // location -> deviceIDs.
    const by_location: Record<string, string[]> = {}

    for (const d of deviceList) {
      by_id[d.device_code] = d;

      let free = true;
      if (d.controller_device_code) {
        (controlled[d.controller_device_code] ??= []).push(d.device_code);
        free = false;
      } else if  (d.attached_to_device_code) {
        free = false; 
      } else {
        switch (d.device_type) {
          case "mining_drone":
            orphan_mining_drones.push(d.device_code);
            break;
          case "survey_drone":
            orphan_survey_drones.push(d.device_code);
            break;
          case "transport_hauler":
          case "transport_drone": orphan_transport_drones.push(d.device_code);
            break;
        }
      }
      if (d.stowed_in_device_code) {
        free = false;
        (stowed[d.stowed_in_device_code] ??= []).push(d.device_code);
      }
      if (free) {
        const loc = d.location || "travelling";
        const system = systemOf(loc);
        (by_system[system] ??= []).push(d.device_code);
        (by_location[loc] ??= []).push(d.device_code);
      }
    }
    Object.entries(by_location).forEach(([loc, deviceIDs]) => {
      deviceIDs.forEach((id, i)=>{
        const others = [...deviceIDs.slice(0, i), ...deviceIDs.slice(i + 1)];
        location_neighbors[id] = others;
      });
    });

    return { controlled, stowed, 
      orphan_mining_drones, orphan_survey_drones, orphan_transport_drones, 
      by_system, by_id, location_neighbors};
  });

  toggle_system_visible(system:string) {
    this.system_visible.update(current=>{
      const next = new Set(current);
      if (next.has(system)){
        next.delete(system);
      } else {
        next.add(system);
      }
      return next;
    });
  } 
}

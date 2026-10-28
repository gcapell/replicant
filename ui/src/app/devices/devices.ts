import { Component, inject, computed, signal, Pipe, PipeTransform } from '@angular/core';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { ReplicantApi } from '../replicant-api';
import { JsonPipe, KeyValuePipe } from '@angular/common';
import { MoveDevice } from '../shared/move-device/move-device';
import { Device } from '../model';
import { humanizeSeconds } from '../format-duration';
import { Adopt } from '../shared/adopt/adopt';
import { Release } from '../shared/release/release';

@Pipe({ name: 'humanizeSeconds', standalone: true })
export class HumanizeSecondsPipe implements PipeTransform {
  transform(value: number): string {
    return humanizeSeconds(value);
  }
}

interface DeviceTree {
  controlled: Record<string, string[]>; // devices controlled by this ID
  stowed: Record<string, string[]>;  // devices stowed in this ID
  orphan_survey_drones: string[];
  orphan_transport_drones: string[];
  orphan_mining_drones: string[];
  by_system: Record<string, string[]>; // System -> device IDs
  by_id: Record<string, Device>;
}

const systemOf = (loc: string): string  => loc?.split("-", 1)[0];

@Component({
  imports: [JsonPipe, MoveDevice, ReactiveFormsModule, HumanizeSecondsPipe, KeyValuePipe, Adopt, Release],
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

    for (const d of deviceList) {
      by_id[d.device_code] = d;
      let free = true;
      if (d.controller_device_code) {
        (controlled[d.controller_device_code] ??= []).push(d.device_code);
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
      if (free && d.location) {
        const system = systemOf(d.location);
        (by_system[system] ??= []).push(d.device_code);
      }
    }
    return { controlled, stowed, 
      orphan_mining_drones, orphan_survey_drones, orphan_transport_drones, 
      by_system, by_id};
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

  setDeliveryDirective(controller: string) {
    if (this.form.invalid) return;
    const v = this.form.getRawValue();
    const requirement = Object.fromEntries(v.items.map(i => [i.resource, i.amount]));
    this.api.setDeliveryDirective(controller, v.collect, v.deliver, requirement);
  }
  surveySystem(controller: string) {
    this.api.setDirective(controller, "survey_system", {
      planets: "all",
      moons: "all",
      recall: true,
    })
  }
  searchBelt(controller: string) {
    this.api.setDirective(controller, "belt_search", {})
  }
  gatherEvenly(controller: string) {
    this.api.setDirective(controller, "gather_evenly", {})
  }
  consolidate(controller: string, dst: string) {
    this.api.setDirective(controller, "consolidate", { deliver: dst })
  }
  setSalvageDirective(controller: string, dst: string) {
    this.api.setDirective(controller, "gather_salvage", {
      recall: false,
      location: dst,
    })
  }
}

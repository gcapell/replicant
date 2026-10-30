import { Component, inject, Pipe, PipeTransform, input } from '@angular/core';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { ReplicantApi } from '../replicant-api';
import { JsonPipe, KeyValuePipe } from '@angular/common';
import { MoveDevice } from '../shared/move-device/move-device';
import { Device } from '../model';
import { Adopt } from '../shared/adopt/adopt';
import { Release } from '../shared/release/release';
import {Directive} from '../shared/directive/directive';
import { humanizeSeconds } from '../format-duration';

@Pipe({ name: 'humanizeSeconds', standalone: true })
export class HumanizeSecondsPipe implements PipeTransform {
  transform(value: number): string {
    return humanizeSeconds(value);
  }
}
export interface DeviceTree {
  controlled: Record<string, string[]>; // devices controlled by this ID
  stowed: Record<string, string[]>;  // devices stowed in this ID
  orphan_survey_drones: string[];
  orphan_transport_drones: string[];
  orphan_mining_drones: string[];
  by_system: Record<string, string[]>; // System -> device IDs
  by_id: Record<string, Device>;
}

@Component({
  imports: [
        MoveDevice, ReactiveFormsModule, JsonPipe,
        KeyValuePipe, Adopt, Release,Directive, HumanizeSecondsPipe,
  ],
  selector: 'app-device',
  styleUrl: './device.scss',
  templateUrl: './device.html',
})
export class DeviceComponent {
  protected readonly api = inject(ReplicantApi);
  device=input.required<Device>();
  deviceTree = input.required<DeviceTree>();
    private fb = inject(FormBuilder);

  salvageSites(location: string): string[] {
    const planet = this.api.replicantScanResult()?.planets.find(
      p => p.designation === location
    );
    return planet?.salvage
      ?.filter(s => !s.depleted)
      .map(s => s.designation) ?? [];
  }

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

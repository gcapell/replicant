import { Component, inject,signal } from '@angular/core';
import { ReplicantApi } from './replicant-api';
import {Tutorials} from './tutorials/tutorials';
import {Messages} from './messages/messages';
import {Events} from './events/events';
import {AccountData} from './account-data/account-data';
import {Location} from './location/location';
import {Replicant} from './replicant/replicant';
import { Blueprints } from './blueprints/blueprints';
import {Devices} from './devices/devices';
import {Locations} from './locations/locations';
import {SystemScan} from './system-scan/system-scan';
import { JsonPipe } from '@angular/common';

@Component({
  imports: [ JsonPipe,Tutorials,Messages,Events,AccountData,Location,Replicant,Blueprints,Devices,SystemScan,Locations],
  selector: 'app-root',
  styleUrl: './app.scss',
  templateUrl: './app.html',
})
export class App {
  protected readonly api = inject(ReplicantApi);
  protected readonly title = signal('Replicant UI');
}

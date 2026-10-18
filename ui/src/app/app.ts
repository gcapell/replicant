import { Component, inject,signal } from '@angular/core';
import { ReplicantApi } from './replicant-api';
import {Tutorials} from './tutorials/tutorials';
import {Messages} from './messages/messages';
import {Events} from './events/events';
import {AccountData} from './account-data/account-data';
import {Location} from './location/location';
import {Replicant} from './replicant/replicant';

@Component({
  imports: [ Tutorials,Messages,Events,AccountData,Location,Replicant],
  selector: 'app-root',
  styleUrl: './app.scss',
  templateUrl: './app.html',
})
export class App {
  protected readonly api = inject(ReplicantApi);
  protected readonly title = signal('Replicant UI');
}

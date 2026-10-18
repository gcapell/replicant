import { Component, inject } from '@angular/core';
import { JsonPipe } from '@angular/common';
import { ReplicantApi } from '../replicant-api';
import {Tutorials} from '../tutorials/tutorials';
import {Messages} from '../messages/messages';
import {Events} from '../events/events';
import {AccountData} from '../account-data/account-data';
import {Location} from '../location/location';

@Component({
  imports: [JsonPipe,Tutorials,Messages,Events,AccountData,Location],
  selector: 'app-user',
  styleUrl: './user.scss',
  templateUrl: './user.html',
})
export class User {
  protected readonly api = inject(ReplicantApi);
}

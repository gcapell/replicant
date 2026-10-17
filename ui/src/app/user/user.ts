import { Component, inject } from '@angular/core';
import { JsonPipe } from '@angular/common';
import { ReplicantApi } from '../replicant-api';
import {Tutorials} from '../tutorials/tutorials';

@Component({
  imports: [JsonPipe,Tutorials],
  selector: 'app-user',
  styleUrl: './user.scss',
  templateUrl: './user.html',
})
export class User {
  protected readonly api = inject(ReplicantApi);
}

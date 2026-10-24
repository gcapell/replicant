import { Component, inject } from '@angular/core';
import { ReplicantApi } from '../replicant-api';
import { JsonPipe } from '@angular/common';
import { MoveDevice} from '../shared/move-device/move-device';

@Component({
  imports: [JsonPipe, MoveDevice],
  selector: 'app-devices',
  styleUrl: './devices.scss',
  templateUrl: './devices.html',
})
export class Devices {
  protected readonly api = inject(ReplicantApi);
}

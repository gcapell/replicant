import { Component, inject, input } from '@angular/core';
import { ReplicantApi } from '../../replicant-api';

@Component({
  imports: [],
  selector: 'app-move-device',
  styleUrl: './move-device.scss',
  templateUrl: './move-device.html',
})
export class MoveDevice {
    protected readonly api = inject(ReplicantApi);
    deviceCode = input.required<string>();
}

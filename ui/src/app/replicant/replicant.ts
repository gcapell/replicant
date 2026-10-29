import { Component, inject } from '@angular/core';
import { ReplicantApi } from '../replicant-api';
import { JsonPipe } from '@angular/common';
import { Starmap, Star } from '../starmap/starmap';



@Component({
  imports: [JsonPipe, Starmap],
  selector: 'app-replicant',
  styleUrl: './replicant.scss',
  templateUrl: './replicant.html',
})
export class Replicant {
  protected readonly api = inject(ReplicantApi);

  normalise(stars: Star[]): Star[] {
    if (stars.length === 0) return [];
    const { x: ox, y: oy, z: oz } = stars[0];
    return stars.map(s => ({
      ...s,
      x: s.x - ox,
      y: s.y - oy,
      z: s.z - oz,
    }));
  }
  stars: Star[] = this.normalise([
    { name: 'OTHYONAL', x: -56.4, y: -25.6, z: -153.6 },
    { name: 'ALWRENAN', x: -58.4, y: -26.3, z: -152.3 },
    { name: 'SYRIREON', x: -57.890, y: -24.2, z: -156.8 },
    { name: 'SYLSHENOS', x: -58.9, y: -22.1, z: -153.4 },
    { name: 'MIRETH', x: -54.3, y: -21.8, z: -156.5 },
  ]);
}


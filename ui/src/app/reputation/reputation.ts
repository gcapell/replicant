import { Component, inject, signal } from '@angular/core';
import { DecimalPipe } from '@angular/common';
import { ReplicantApi } from '../replicant-api';
import { SpeciesReputation } from '../model';

@Component({
  imports: [DecimalPipe],
  selector: 'app-reputation',
  styleUrl: './reputation.scss',
  templateUrl: './reputation.html',
})
export class Reputation {
  protected readonly api = inject(ReplicantApi);
  readonly species = signal<SpeciesReputation[] | null>(null);
  readonly loading = signal(false);

  async fetch() {
    this.loading.set(true);
    try {
      const r = await this.api.reputation();
      this.species.set([...r.reputation].sort((a, b) => a.name.localeCompare(b.name)));
    } catch (e) {
      this.api.errorMessage.set(e);
      console.error('Error loading /v1/accounts/reputation:', e);
    } finally {
      this.loading.set(false);
    }
  }
}

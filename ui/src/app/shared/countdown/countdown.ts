import { Component, Input, OnChanges, OnDestroy, signal } from '@angular/core';


@Component({
  imports: [],
  selector: 'app-countdown',
  styleUrl: './countdown.scss',
  template: `<span>{{ text() }}</span>`,
})
export class Countdown implements OnChanges, OnDestroy {
  @Input({ required: true }) deadline!: Date | string | number;

  text = signal('');
  private timeoutId?: ReturnType<typeof setTimeout>;

  ngOnChanges() {
    clearTimeout(this.timeoutId);
    this.tick();
  }
  ngOnDestroy() {
    clearTimeout(this.timeoutId);
  }

  private tick() {
    const diff = new Date(this.deadline).getTime() - Date.now();

    if (diff <= 0) {
      this.text.set('Deadline passed');
      return; // stop scheduling
    }

    const[text, delay] = this.format(diff);
    this.text.set(text);
    this.timeoutId = setTimeout(() => this.tick(), delay);
  }

  private format(diff: number): [string, number] {
    const s = Math.floor(diff / 1000);
    const d = Math.floor(s / 86400);
    const h = Math.floor((s % 86400) / 3600);
    const m = Math.floor((s % 3600) / 60);

    const second = 1000;
    const minute = 60 * second;
    const hour = 60 * minute;

    if (d > 3) {
      return [`{d}d`, 8 * hour];
    }
    if (d > 0) {
      return [`${d}d ${h}h`, 1 * hour];
    }
    if (h > 3) {
      return [`${h}h`, 10 * minute];
    }
    if (h > 0) {
      return [`${h}h ${m}m`, 1 * minute];
    }
    if (m > 5) {
      return [`${m}m`, 10 * second];
    }
    if (m > 0) {
      return [`${m}m ${s%60}s`, 5 * second];
    }
    return [`${s}s`, 1 * second];
  }
}

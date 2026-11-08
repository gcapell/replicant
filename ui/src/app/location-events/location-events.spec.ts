import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { LocationEvents, formatReward, optionSatisfied } from './location-events';

const event = (location: string, tier: number, options: unknown[], rewards: unknown = { resources: {} }) => ({
  location, designation: `${location}-EVT-001`, status: 'active', tier,
  progress: { met: false, replicant_present: false, options },
  rewards,
});

describe('LocationEvents', () => {
  let component: LocationEvents;
  let fixture: ComponentFixture<LocationEvents>;
  let http: HttpTestingController;

  const rows = (): string[][] =>
    [...fixture.nativeElement.querySelectorAll('tbody tr')].map((tr: HTMLElement) =>
      [...tr.querySelectorAll('td')].map((td) => {
        const lines = [...td.querySelectorAll('div')].map((d) => d.textContent?.trim());
        return lines.length ? lines.join('|') : (td.textContent?.trim() ?? '');
      }),
    );

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [LocationEvents],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();

    fixture = TestBed.createComponent(LocationEvents);
    component = fixture.componentInstance;
    http = TestBed.inject(HttpTestingController);
    await fixture.whenStable();
  });

  afterEach(() => http.verify());

  it('shows nothing before Fetch', () => {
    fixture.detectChanges();
    expect(rows()).toEqual([]);
    expect(fixture.nativeElement.textContent).not.toContain('No active location events.');
  });

  it('lists active events from /v1/accounts/events, sorted, across pages', async () => {
    const done = component.fetch();
    http.expectOne('/v1/accounts/events?status=active').flush({
      events: [
        event('ZED-2', 1, [
          // met is false (no replicant), but everything is there: ticked.
          { name: 'default', met: false, devices: [],
            resources: [{ resource_type: 'structural', current: 565, required: 350, met: true }] },
        ], { resources: { volatiles: 200 } }),
      ],
      next_cursor: 20,
    });
    await new Promise((r) => setTimeout(r));
    http.expectOne('/v1/accounts/events?status=active&cursor=20').flush({
      events: [
        event('ABA-3', 2, [
          {
            name: 'a', met: false,
            resources: [{ resource_type: 'conductive', current: 100, required: 100, met: true }],
            devices: [{ device_type: 'defence_grid', current: 0, required: 1, met: false }],
          },
          { name: 'b', met: false, resources: [], devices: [{ device_type: 'shield_generator', current: 1, required: 1, met: true }] },
          {
            name: 'c', met: false,
            resources: [
              { resource_type: 'carbon', current: 0, required: 100, met: false },
              { resource_type: 'silicates', current: 25, required: 250, met: false },
            ],
            devices: [],
          },
        ], { xp: 10, civilisation_points: 5, resources: { volatiles: 7, carbon: 500, rares: 0 } }),
      ],
      next_cursor: null,
    });
    await done;
    fixture.detectChanges();
    expect(rows()).toEqual([
      [
        'ABA-3',
        '2',
        'Co(100/100), defence_grid(0/1)|shield_generator(1/1) ✅|C(0/100), Si(25/250)',
        'C500, V7',
      ],
      ['ZED-2', '1', 'S(565/350) ✅', 'V200'],
    ]);
  });

  it('says so when there are no active events', async () => {
    const done = component.fetch();
    http.expectOne('/v1/accounts/events?status=active').flush({ events: [], next_cursor: null });
    await done;
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain('No active location events.');
  });

  it('is satisfied only when every resource and device is there', () => {
    const r = (current: number, required: number) => ({ resource_type: 'carbon', current, required, met: false });
    const d = (current: number, required: number) => ({ device_type: 'x', current, required, met: false });
    expect(optionSatisfied({ name: '', met: false, resources: [r(5, 5)], devices: [d(2, 1)] })).toBe(true);
    expect(optionSatisfied({ name: '', met: false, resources: [r(4, 5)], devices: [d(1, 1)] })).toBe(false);
    expect(optionSatisfied({ name: '', met: true, resources: [], devices: [d(0, 1)] })).toBe(false);
  });

  it('abbreviates all six resources', () => {
    expect(
      formatReward({
        civilisation_points: 0, completion_achievement: '', xp: 0,
        resources: { volatiles: 6, structural: 5, silicates: 4, rares: 3, conductive: 2, carbon: 1 },
      }),
    ).toBe('C1, Co2, R3, Si4, S5, V6');
  });
});

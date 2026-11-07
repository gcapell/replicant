import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { LocationEvents, formatReward } from './location-events';
import { ReplicantApi } from '../replicant-api';

const summary = (events: number) => ({
  devices: 0, location_events: events, replicants: 0, resource_sites: 0, resources: 0,
});

describe('LocationEvents', () => {
  let component: LocationEvents;
  let fixture: ComponentFixture<LocationEvents>;
  let http: HttpTestingController;
  let api: ReplicantApi;

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
    api = TestBed.inject(ReplicantApi);
    await fixture.whenStable();
  });

  it('lists event locations from /v1/locations, sorted, before any details are fetched', () => {
    api.locationList.set({
      locations: { 'ZED-2': summary(1), 'ABA-1': summary(0), 'ABA-3-L4': summary(1) },
    });
    fixture.detectChanges();
    expect(rows()).toEqual([
      ['ABA-3-L4', '', '', ''],
      ['ZED-2', '', '', ''],
    ]);
  });

  it('fills in tier, request and reward on Fetch', async () => {
    api.locationList.set({ locations: { 'ABA-3': summary(1) } });
    const done = component.fetch();
    http.expectOne('/v1/locations/ABA-3').flush({
      location: 'ABA-3',
      location_event: {
        tier: 2,
        progress: {
          options: [
            {
              name: 'a', met: false,
              resources: [{ resource_type: 'conductive', current: 0, required: 100, met: false }],
              devices: [{ device_type: 'defence_grid', current: 0, required: 1, met: false }],
            },
            { name: 'b', met: true, resources: [], devices: [{ device_type: 'shield_generator', current: 1, required: 1, met: true }] },
            {
              name: 'c', met: false,
              resources: [
                { resource_type: 'carbon', current: 0, required: 100, met: false },
                { resource_type: 'silicates', current: 25, required: 250, met: false },
              ],
              devices: [],
            },
          ],
        },
        rewards: { xp: 10, civilisation_points: 5, resources: { volatiles: 7, carbon: 500, rares: 0 } },
      },
    });
    await done;
    fixture.detectChanges();
    expect(rows()).toEqual([
      [
        'ABA-3',
        '2',
        'Co(0/100), defence_grid(0/1)|shield_generator(1/1) ✅|C(0/100), Si(25/250)',
        'C500, V7',
      ],
    ]);
  });

  it('fetches /v1/locations first if Locations has not been fetched', async () => {
    const done = component.fetch();
    http.expectOne('/v1/locations').flush({ locations: { 'ABA-1': summary(0) } });
    await done;
    http.verify();
    fixture.detectChanges();
    expect(fixture.nativeElement.textContent).toContain('No location events.');
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

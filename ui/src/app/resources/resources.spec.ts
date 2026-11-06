import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { Resources, tabulate } from './resources';

describe('Resources', () => {
  let component: Resources;
  let fixture: ComponentFixture<Resources>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [Resources],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();

    fixture = TestBed.createComponent(Resources);
    component = fixture.componentInstance;
    await fixture.whenStable();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});

describe('tabulate', () => {
  it('sums each star across its locations, in either items format', () => {
    const t = tabulate([
      { location: 'MIRETH-5', items: [{ resource_type: 'silicates', quantity: 10 }] },
      { location: 'MIRETH-BELT-1', items: { silicates: 5, carbon: 7 } },
      { location: 'BETSI-2-1', items: [{ resource_type: 'rares', quantity: 3 }] },
    ]);
    expect(t.columns).toEqual(['carbon', 'rares', 'silicates']);
    expect(t.rows).toEqual([
      { star: 'BETSI', amounts: { rares: 3 } },
      { star: 'MIRETH', amounts: { silicates: 15, carbon: 7 } },
    ]);
  });
});

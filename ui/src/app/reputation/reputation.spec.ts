import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Reputation } from './reputation';

describe('Reputation', () => {
  let component: Reputation;
  let fixture: ComponentFixture<Reputation>;
  let http: HttpTestingController;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [Reputation],
      providers: [provideHttpClient(), provideHttpClientTesting()],
    }).compileComponents();

    fixture = TestBed.createComponent(Reputation);
    component = fixture.componentInstance;
    http = TestBed.inject(HttpTestingController);
    await fixture.whenStable();
  });

  it('shows a row per species, sorted by name', async () => {
    const done = component.fetch();
    http.expectOne('/v1/accounts/reputation').flush({
      reputation: [
        { name: 'Velthari', species_key: 'velthari', description: '', trait: 'diplomatic', total_reputation: 3.0 },
        { name: 'Fendrith', species_key: 'fendrith', description: '', trait: 'curious', total_reputation: 9.0 },
      ],
    });
    await done;
    fixture.detectChanges();
    const rows = [...fixture.nativeElement.querySelectorAll('tbody tr')].map((tr: HTMLElement) =>
      [...tr.querySelectorAll('td')].map((td) => td.textContent?.trim()),
    );
    expect(rows).toEqual([
      ['Fendrith', '9'],
      ['Velthari', '3'],
    ]);
  });
});

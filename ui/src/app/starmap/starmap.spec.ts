import { ComponentFixture, TestBed } from '@angular/core/testing';
import { Starmap } from './starmap';

describe('Starmap', () => {
  let component: Starmap;
  let fixture: ComponentFixture<Starmap>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [Starmap],
    }).compileComponents();

    fixture = TestBed.createComponent(Starmap);
    component = fixture.componentInstance;
    await fixture.whenStable();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});

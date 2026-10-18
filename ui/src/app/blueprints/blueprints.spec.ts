import { ComponentFixture, TestBed } from '@angular/core/testing';
import { Blueprints } from './blueprints';

describe('Blueprints', () => {
  let component: Blueprints;
  let fixture: ComponentFixture<Blueprints>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [Blueprints],
    }).compileComponents();

    fixture = TestBed.createComponent(Blueprints);
    component = fixture.componentInstance;
    await fixture.whenStable();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});

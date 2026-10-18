import { ComponentFixture, TestBed } from '@angular/core/testing';
import { Replicant } from './replicant';

describe('Replicant', () => {
  let component: Replicant;
  let fixture: ComponentFixture<Replicant>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [Replicant],
    }).compileComponents();

    fixture = TestBed.createComponent(Replicant);
    component = fixture.componentInstance;
    await fixture.whenStable();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});

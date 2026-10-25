import { ComponentFixture, TestBed } from '@angular/core/testing';
import { Adopt } from './adopt';

describe('Adopt', () => {
  let component: Adopt;
  let fixture: ComponentFixture<Adopt>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [Adopt],
    }).compileComponents();

    fixture = TestBed.createComponent(Adopt);
    component = fixture.componentInstance;
    await fixture.whenStable();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});

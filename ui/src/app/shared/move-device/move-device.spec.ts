import { ComponentFixture, TestBed } from '@angular/core/testing';
import { MoveDevice } from './move-device';

describe('MoveDevice', () => {
  let component: MoveDevice;
  let fixture: ComponentFixture<MoveDevice>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [MoveDevice],
    }).compileComponents();

    fixture = TestBed.createComponent(MoveDevice);
    component = fixture.componentInstance;
    await fixture.whenStable();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});

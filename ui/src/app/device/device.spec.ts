import { ComponentFixture, TestBed } from '@angular/core/testing';
import { Device } from './device';

describe('Device', () => {
  let component: Device;
  let fixture: ComponentFixture<Device>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [Device],
    }).compileComponents();

    fixture = TestBed.createComponent(Device);
    component = fixture.componentInstance;
    await fixture.whenStable();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});

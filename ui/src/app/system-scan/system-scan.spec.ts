import { ComponentFixture, TestBed } from '@angular/core/testing';
import { SystemScan } from './system-scan';

describe('SystemScan', () => {
  let component: SystemScan;
  let fixture: ComponentFixture<SystemScan>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [SystemScan],
    }).compileComponents();

    fixture = TestBed.createComponent(SystemScan);
    component = fixture.componentInstance;
    await fixture.whenStable();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});

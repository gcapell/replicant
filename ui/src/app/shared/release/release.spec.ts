import { ComponentFixture, TestBed } from '@angular/core/testing';
import { Release } from './release';

describe('Release', () => {
  let component: Release;
  let fixture: ComponentFixture<Release>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [Release],
    }).compileComponents();

    fixture = TestBed.createComponent(Release);
    component = fixture.componentInstance;
    await fixture.whenStable();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});

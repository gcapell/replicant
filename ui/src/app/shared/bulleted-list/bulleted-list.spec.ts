import { ComponentFixture, TestBed } from '@angular/core/testing';
import { BulletedList } from './bulleted-list';

describe('BulletedList', () => {
  let component: BulletedList;
  let fixture: ComponentFixture<BulletedList>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [BulletedList],
    }).compileComponents();

    fixture = TestBed.createComponent(BulletedList);
    component = fixture.componentInstance;
    await fixture.whenStable();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});

import { ComponentFixture, TestBed } from '@angular/core/testing';
import { AccountData } from './account-data';

describe('AccountData', () => {
  let component: AccountData;
  let fixture: ComponentFixture<AccountData>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [AccountData],
    }).compileComponents();

    fixture = TestBed.createComponent(AccountData);
    component = fixture.componentInstance;
    await fixture.whenStable();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});

import { ComponentFixture, TestBed } from '@angular/core/testing';
import { KvTable } from './kv-table';

describe('KvTable', () => {
  let component: KvTable;
  let fixture: ComponentFixture<KvTable>;

  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [KvTable],
    }).compileComponents();

    fixture = TestBed.createComponent(KvTable);
    component = fixture.componentInstance;
    await fixture.whenStable();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });
});

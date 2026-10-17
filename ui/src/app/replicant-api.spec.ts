import { TestBed } from '@angular/core/testing';
import { ReplicantApi } from './replicant-api';

describe('ReplicantApi', () => {
  let service: ReplicantApi;

  beforeEach(() => {
    TestBed.configureTestingModule({});
    service = TestBed.inject(ReplicantApi);
  });

  it('should be created', () => {
    expect(service).toBeTruthy();
  });
});

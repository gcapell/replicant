import { Service, signal, inject, WritableSignal } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { AccountData, Replicant, Location , SearchResult} from './model';

@Service()
export class ReplicantApi {
    readonly token = signal(localStorage.getItem('token') ?? '');
    protected readonly http = inject(HttpClient);

    readonly accountData = signal<AccountData | null>(null);
    readonly replicant = signal<Replicant | null>(null);
    readonly location = signal<Location | null>(null);
    readonly errorMessage = signal<unknown>(null);
    readonly searchResult = signal<SearchResult | null>(null);

    private fetchAndUpdate<T>(path: string, target: WritableSignal<T | null>) {
        this.http.get<T>(path, {
            headers: {
                'Authorization': `Bearer ${this.token()}`
            }
        }).subscribe({
            next: (data) => {
                target.set(data);
            },
            error: (error) => {
                this.errorMessage.set(error);
                target.set(null);
                console.error(`Error loading ${path}:`, error);
            }
        });
    }

    private postAndUpdate<T>(path: string, body: any, target: WritableSignal<T | null>) {
        this.http.post<T>(path, body, {
            headers: {
                'Authorization': `Bearer ${this.token()}`,
                'Content-Type': 'application/json',
            }
        }).subscribe({
            next: (data) => {
                target.set(data);
            },
            error: (error) => {
                this.errorMessage.set(error);
                target.set(null);
                console.error(`Error posting to ${path}:`, error);
            }
        });
    }


    loadReplicant(id: string) {
        this.fetchAndUpdate(`/v1/replicants/${id}`, this.replicant);
    }
    loadLocation(id: string) {
        this.fetchAndUpdate(`/v1/locations/${id}`, this.location);
    }
    loadAccount(token: string) {
        localStorage.setItem('token', token);
        this.token.set(token);
        this.fetchAndUpdate(`/v1/accounts/me`, this.accountData);
    }
    search(id: string) {
        this.postAndUpdate(
            `/v1/devices/${id}`, 
            {"command": "search"}, 
            this.searchResult);
    }
}

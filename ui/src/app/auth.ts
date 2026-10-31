import { Injectable, inject, signal } from '@angular/core';
import { HttpInterceptorFn } from '@angular/common/http';

@Injectable({ providedIn: 'root' })
export class AuthService {
    readonly token = signal<string | null>(null);
}

const API_ORIGIN = '/v1';

export const authInterceptor: HttpInterceptorFn = (req, next) => {
    const token = inject(AuthService).token();
    if (!token || !req.url.startsWith(API_ORIGIN)) return next(req);
    return next(req.clone({ setHeaders: { Authorization: `Bearer ${token}` } }));
};

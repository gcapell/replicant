import { Component, signal , inject } from '@angular/core';
import { User } from './user/user';


@Component({
  imports: [ User],
  selector: 'app-root',
  styleUrl: './app.scss',
  templateUrl: './app.html',
})
export class App {
  protected readonly title = signal('Replicant UI');
}

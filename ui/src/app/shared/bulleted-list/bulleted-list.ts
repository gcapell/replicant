import { Component, input } from '@angular/core';

@Component({
  imports: [],
  selector: 'app-bulleted-list',
  styleUrl: './bulleted-list.scss',
  templateUrl: './bulleted-list.html',
})
export class BulletedList {
   items = input<string[] | null | undefined>();
}

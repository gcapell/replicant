import { Component,input, computed } from '@angular/core';
import { JsonPipe } from '@angular/common';
import {AMIDirective} from '../../model'

@Component({
  imports: [JsonPipe],
  selector: 'app-directive',
  styleUrl: './directive.scss',
  templateUrl: './directive.html',
})
export class Directive {
  directive = input.required<AMIDirective>();
  hasConfig = computed(()=>{
    const c = this.directive().config;
    return !!c && Object.keys(c).length > 0;
  });
}

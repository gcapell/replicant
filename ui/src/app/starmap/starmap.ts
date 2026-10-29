import {
  Component, ElementRef, Input, OnChanges, OnDestroy,
  AfterViewInit, SimpleChanges, ViewChild, NgZone
} from '@angular/core';
import * as THREE from 'three';
import { OrbitControls } from 'three/examples/jsm/controls/OrbitControls.js';
import { CSS2DRenderer, CSS2DObject } from 'three/examples/jsm/renderers/CSS2DRenderer.js';

export interface Star {
  name: string;
  x: number;
  y: number;
  z: number;
}


@Component({
  imports: [],
  selector: 'app-starmap',
  template: `<div #container class="container"></div>`,
  styles: [`
    .container { width: 100%; height: 100%; position: relative; }
    :host { display: block; width: 100%; height: 100%; }
  `]
})
export class Starmap implements AfterViewInit, OnChanges, OnDestroy {
  @ViewChild('container') containerRef!: ElementRef<HTMLDivElement>;
  @Input() stars: Star[] = [];

  private renderer!: THREE.WebGLRenderer;
  private scene = new THREE.Scene();
  private camera!: THREE.PerspectiveCamera;
  private controls!: OrbitControls;
  private points?: THREE.Points;
  private raycaster = new THREE.Raycaster();
  private mouse = new THREE.Vector2();
  private frameId = 0;
  private resizeObserver!: ResizeObserver;
  private labelRenderer!: CSS2DRenderer;
  private labelGroup = new THREE.Group();

  constructor(private zone: NgZone) { }

  ngAfterViewInit() {
    const el = this.containerRef.nativeElement;
    const { clientWidth: w, clientHeight: h } = el;

    this.camera = new THREE.PerspectiveCamera(60, w / h, 0.1, 100000);
    this.camera.position.set(0, 0, 200);

    this.renderer = new THREE.WebGLRenderer({ antialias: true });
    this.renderer.setSize(w, h);
    this.renderer.setPixelRatio(window.devicePixelRatio);
    el.appendChild(this.renderer.domElement);

    this.controls = new OrbitControls(this.camera, this.renderer.domElement);
    this.controls.enableDamping = true;

    this.scene.background = new THREE.Color(0x000008);
    this.scene.add(new THREE.AxesHelper(50)); // handy for orientation

    this.resizeObserver = new ResizeObserver(() => this.onResize());
    this.resizeObserver.observe(el);


    this.labelRenderer = new CSS2DRenderer();
    this.labelRenderer.setSize(w, h);
    const ld = this.labelRenderer.domElement;
    ld.style.position = 'absolute';
    ld.style.top = '0';
    ld.style.left = '0';
    ld.style.pointerEvents = 'none'; // let mouse events fall through to the canvas / OrbitControls
    el.appendChild(ld);

    this.scene.add(this.labelGroup);

    this.buildPoints();

    // Run the render loop outside Angular so it doesn't trigger change detection every frame
    this.zone.runOutsideAngular(() => this.animate());
  }

  ngOnChanges(changes: SimpleChanges) {
    if (changes['stars'] && this.renderer) this.buildPoints();
  }

  private buildPoints() {
    if (this.points) {
      this.scene.remove(this.points);
      this.points.geometry.dispose();
      (this.points.material as THREE.Material).dispose();
    }
    const positions = new Float32Array(this.stars.length * 3);
    this.stars.forEach((s, i) => {
      positions[i * 3] = s.x;
      positions[i * 3 + 1] = s.y;
      positions[i * 3 + 2] = s.z;
    });
    const geom = new THREE.BufferGeometry();
    geom.setAttribute('position', new THREE.BufferAttribute(positions, 3));
    const mat = new THREE.PointsMaterial({ color: 0xffffff, size: 4, sizeAttenuation: false });
    this.points = new THREE.Points(geom, mat);
    this.scene.add(this.points);

    this.labelGroup.clear(); // removes the old labels (and their DOM elements)
    for (const s of this.stars) {
      const div = document.createElement('div');
      div.textContent = s.name;
      div.style.color = '#cde';
      div.style.fontSize = '11px';
      div.style.fontFamily = 'sans-serif';
      div.style.textShadow = '0 0 3px #000';
      div.style.whiteSpace = 'nowrap';
      div.style.transform = 'translate(8px, -50%)'; // nudge it off the dot
      const label = new CSS2DObject(div);
      label.position.set(s.x, s.y, s.z);
      this.labelGroup.add(label);
    }

    // Frame the data
    geom.computeBoundingSphere();
    const bs = geom.boundingSphere;
    if (bs) {
      this.controls.target.copy(bs.center);
      this.camera.position.copy(bs.center).add(new THREE.Vector3(0, 0, bs.radius * 2.5 + 1));
      this.controls.update();
    }
  }

  private onResize() {
    const el = this.containerRef.nativeElement;
    this.camera.aspect = el.clientWidth / el.clientHeight;
    this.camera.updateProjectionMatrix();
    this.renderer.setSize(el.clientWidth, el.clientHeight);
    this.labelRenderer.setSize(el.clientWidth, el.clientHeight);
  }

  private animate = () => {
    this.frameId = requestAnimationFrame(this.animate);
    this.controls.update();
    this.renderer.render(this.scene, this.camera);
    this.labelRenderer.render(this.scene, this.camera);
  };

  ngOnDestroy() {
    cancelAnimationFrame(this.frameId);
    this.resizeObserver?.disconnect();
    this.controls.dispose();
    this.points?.geometry.dispose();
    this.renderer.dispose();
    this.labelRenderer.domElement.remove();
  }
}
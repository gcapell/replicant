import { Service, signal, inject, WritableSignal, computed } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import {
    AccountData, ScanDroneResult,
    DeviceList, Replicant, Location, MessageList,
    Tutorials, TutorialDetail, SendDroneResult,
    StartMiningResult, ReplicantScanResult, BlueprintsList,
    StartPrintingResult, FillTransportResult, EmptyTransportResult, AdoptDronesResult,
    SetDeliveryDirectiveResult, CompleteEventResult, SetDirectiveResult, ReleaseDronesResult,
    PatrolResult, MarkReadResult, LocationList, LocationSummary,
    CommandResult, Device, InventoryList, LocationInventory, ReputationList,
} from './model';
import { EventList } from './eventmodel';
import { firstValueFrom } from 'rxjs';
import { AuthService } from './auth';

type Params = Record<string, string | number | boolean>;

function pushTo<K, V>(m: Map<K, V[]>, key: K, value: V): void {
    const list = m.get(key);
    if (list) list.push(value);
    else m.set(key, [value]);
}

export const systemOf = (loc: string): string => loc?.split("-", 1)[0];

function setSignal<K, V>(m: Map<K, WritableSignal<V>>, key: K, val: V) {
    let s = m.get(key);
    if (s) {
        s.set(val);
    } else {
        s = signal<V>(val);
        m.set(key, s);
    }
}

type DeviceCommand = 'prospect' | 'activate' | 'deactivate' | 'scan' | 'stow' | 'deploy' | 'compact' | 'unfurl' |
    'travel' | 'start_mining' | 'collect_resources' | 'deposit_resources' | 'adopt' | 'release' |
    'set_directive' | 'attach' | 'detach' | 'enqueue_print';

export interface SystemSummary {
    summary: LocationSummary;
    children: Map<string, LocationSummary>;
}

const LOCATION_SUMMARY_KEYS = [
    'devices', 'location_events', 'replicants', 'resource_sites', 'resources',
] as const satisfies readonly (keyof LocationSummary)[];

function emptySummary(): LocationSummary {
    return { devices: 0, location_events: 0, replicants: 0, resource_sites: 0, resources: 0 };
}

function addSummary(target: LocationSummary, add: LocationSummary): void {
    for (const k of LOCATION_SUMMARY_KEYS) target[k] += add[k];
}

export function groupBySystem(list: LocationList): Map<string, SystemSummary> {
    const systems = new Map<string, SystemSummary>();
    for (const [loc, summary] of Object.entries(list.locations)) {
        const name = systemOf(loc);
        let s = systems.get(name);
        if (!s) {
            s = { children: new Map(), summary: emptySummary() };
            systems.set(name, s);
        }
        s.children.set(loc, summary);
        addSummary(s.summary, summary);
    }
    return systems;
}

@Service()
export class ReplicantApi {
    readonly token = signal(localStorage.getItem('token') ?? '');
    protected readonly http = inject(HttpClient);
    protected readonly auth = inject(AuthService);

    readonly accountData = signal<AccountData | null>(null);
    readonly replicant = signal<Replicant | null>(null);
    readonly location = signal<Location | null>(null);
    readonly errorMessage = signal<unknown>(null);
    readonly tutorialList = signal<Tutorials | null>(null);
    readonly tutorialDetail = signal<TutorialDetail | null>(null);
    readonly messageList = signal<MessageList | null>(null);
    readonly eventList = signal<EventList | null>(null);
    readonly startMiningResult = signal<StartMiningResult | null>(null);
    readonly replicantScanResult = signal<ReplicantScanResult | null>(null);
    readonly blueprintList = signal<BlueprintsList | null>(null);
    readonly deviceList = signal<DeviceList | null>(null);
    readonly sendDroneResult = signal<SendDroneResult | null>(null);
    readonly scanDroneResult = signal<ScanDroneResult | null>(null);
    readonly startPrintingResult = signal<StartPrintingResult | null>(null);
    readonly fillTransportResult = signal<FillTransportResult | null>(null);
    readonly emptyTransportResult = signal<EmptyTransportResult | null>(null);
    readonly adoptDronesResult = signal<AdoptDronesResult | null>(null);
    readonly setDeliveryDirectiveResult = signal<SetDeliveryDirectiveResult | null>(null);
    readonly completeEventResult = signal<CompleteEventResult | null>(null);
    readonly setDirectiveResult = signal<SetDirectiveResult | null>(null);
    readonly releaseDronesResult = signal<ReleaseDronesResult | null>(null);
    readonly patrolResult = signal<PatrolResult | null>(null);
    readonly markReadResult = signal<MarkReadResult | null>(null);
    readonly locationList = signal<LocationList | null>(null);
    readonly commandResult = signal<CommandResult | null>(null);

    id2device: Map<string, WritableSignal<Device>> = new Map();          // deviceId -> Device
    system2deviceIDs: Map<string, WritableSignal<string[]>> = new Map();  // systemId -> deviceIDs.  Excludes controlled/stowed devices.
    location2deviceIDs: Map<string, WritableSignal<string[]>> = new Map();  // locationID -> deviceIDs.  Excludes controlled/stowed devices.
    controlled: Map<string, WritableSignal<string[]>> = new Map();   // deviceID -> ids of devices controlled
    systemScans: Map<string, WritableSignal<ReplicantScanResult>> = new Map();

    // system -> ids of controllable devices not currently being controlled.
    controllableMiningDrones: Map<string, WritableSignal<string[]>> = new Map();
    controllableSurveyDrones: Map<string, WritableSignal<string[]>> = new Map();
    controllableTransports: Map<string, WritableSignal<string[]>> = new Map();

    devicesInSystem(system: string): WritableSignal<string[]> {
        let ids = this.system2deviceIDs.get(system);
        if (!ids) {
            ids = signal<string[]>([]);
            this.system2deviceIDs.set(system, ids);
        }
        return ids;
    }
    readonly systems = computed(() => {
        const list = this.locationList();
        return list ? groupBySystem(list) : new Map<string, SystemSummary>();
    });

    readonly systemNames = computed(() => Array.from(this.systems().keys()));

    async locations() {
        try {
            this.locationList.set(
                await firstValueFrom(this.http.get<LocationList>('/v1/locations'))
            );
        } catch (err) {
            console.error('Failed to load locations', err);
            // set an error signal / show a message here
        }
        for (const id of this.systemNames()) {
            if (!this.systemScans.has(id)) {
                const systemDetails = await firstValueFrom(this.http.get<ReplicantScanResult>(`/v1/locations/${id}`));
                console.log(id, "has", systemDetails.planets.length, "planets");
                setSignal(this.systemScans, id, systemDetails);
            }
        }
    }

    async listDevicesAsync(system: string) {
        const ids: string[] = [];
        const controlled: Map<string, string[]> = new Map();
        const controllableMining: string[] = [];
        const controllableSurvey: string[] = [];
        const controllableTransport: string[] = [];
        const location2ids: Map<string, string[]> = new Map();

        for await (const dl of this.pagedGet<DeviceList>(`/v1/devices`, { location: system })) {
            for (const d of dl.devices) {
                console.log("device", d.device_type, d.device_code, d.location);
                setSignal(this.id2device, d.device_code, d);
                let free = true;
                if (d.controller_device_code) {
                    pushTo(controlled, d.controller_device_code, d.device_code);
                    free = false
                } else {
                    switch (d.device_type) {
                        case "mining_drone":
                            controllableMining.push(d.device_code);
                            break;
                        case "survey_drone":
                        case "belt_surveyor":
                            controllableSurvey.push(d.device_code);
                            break;
                    }
                    if (d.features.includes('transport')) {
                        controllableTransport.push(d.device_code);
                    }
                }
                if (d.attached_to_device_code) {
                    free = false;
                }
                if (free) {
                    ids.push(d.device_code);
                    pushTo(location2ids, d.location, d.device_code);
                }
            }
        }
        setSignal(this.system2deviceIDs, system, ids);
        setSignal(this.controllableMiningDrones, system, controllableMining);
        setSignal(this.controllableSurveyDrones, system, controllableSurvey);
        setSignal(this.controllableTransports, system, controllableTransport);
        for (const [loc, ids] of location2ids) {
            setSignal(this.location2deviceIDs, loc, ids);
        }
        for (const [controller, ids] of controlled) {
            setSignal(this.controlled, controller, ids);
        }
        console.log("system2deviceIDs", this.system2deviceIDs);
    }

    async *pagedGet<T extends { next_cursor?: number | string | null }>(
        url: string,
        params: Params = {},
    ): AsyncGenerator<T> {
        let cursor: string | number | null | undefined;
        do {
            console.log("cursor", cursor);
            const page = await firstValueFrom(
                this.http.get<T>(url, { params: cursor != null ? { ...params, cursor } : params }),
            );
            yield page;
            cursor = page.next_cursor;
        } while (cursor != null);
    }

    // All our stockpiles, every page of GET /v1/inventory.
    async inventory(): Promise<LocationInventory[]> {
        const all: LocationInventory[] = [];
        for await (const page of this.pagedGet<InventoryList>('/v1/inventory', { limit: 50 })) {
            all.push(...page.locations);
        }
        return all;
    }

    async reputation(): Promise<ReputationList> {
        return firstValueFrom(this.http.get<ReputationList>('/v1/accounts/reputation'));
    }

    private fetchAndUpdate<T>(path: string, target: WritableSignal<T | null>) {
        console.log("GET", path);
        this.http.get<T>(path).subscribe({
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

    private fetchAllPages<T extends { next_cursor?: number | null }>(
        path: string,
        target: WritableSignal<T | null>,
        merge: (soFar: T, page: T) => T,
    ) {
        target.set(null);                       // clear once, up front
        this.fetchPage(path, target, merge, null);
    }

    private fetchPage<T extends { next_cursor?: number | null }>(
        path: string,
        target: WritableSignal<T | null>,
        merge: (soFar: T, page: T) => T,
        cursor: number | null,
    ) {
        console.log("GET", path);
        this.http.get<T>(path, {
            params: cursor !== null ? { cursor } : {},
        }).subscribe({
            next: (page) => {
                target.update(cur => cur ? merge(cur, page) : page);
                if (page.next_cursor != null) {
                    this.fetchPage(path, target, merge, page.next_cursor);
                }
            },
            error: (e) => {
                this.errorMessage.set(e);
                target.set(null);
                console.error(`Error loading ${path}:`, e);
            },
        });
    }

    private postAndUpdate<T>(path: string, body: any, target: WritableSignal<T | null>) {
        console.log("POST", body, "to", path);
        this.http.post<T>(path, body, {
            headers: {
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

    async asyncPost<T>(path: string, body: any): Promise<T> {
        return firstValueFrom(this.http.post<T>(path, body, {
            headers: {
                'Content-Type': 'application/json',
            }
        }));
    }

    async collectResources(drone: string, resource: string, amount: string) {
        const r = await this.asyncPost<FillTransportResult>(`/v1/devices/${drone}`, {
            command: "collect_resources",
            resources: { [resource]: Number(amount) },
        });
        this.fillTransportResult.set(r);
        this.refreshDevice(drone);
    }

    async refreshDevice(id: string) {
        const d = await firstValueFrom(this.http.get<Device>(`/v1/devices/${id}`)  );
        setSignal(this.id2device, id, d);
    }

    systemOf = (loc: string): string => loc?.split("-", 1)[0];

    loadReplicant(id: string) {
        this.fetchAndUpdate(`/v1/replicants/${id}`, this.replicant);
    }

    // system -> salvage sites
    salvageSites = computed<Record<string, string[]>>(() => {
        const reply: Record<string, string[]> = {};
        Object.entries(this.locationList()?.locations ?? {})
            .forEach(([loc, details], i) => {
                if (details.resource_sites) {
                    (reply[this.systemOf(loc)] ??= []).push(loc);
                }
            });
        console.log("salvageSites", reply);
        return reply;
    });



    loadLocation(id: string) {
        this.fetchAndUpdate(`/v1/locations/${id}`, this.location);
    }
    listTutorials() {
        this.fetchAndUpdate(`/v1/tutorials`, this.tutorialList);
    }
    loadTutorial(slug: string) {
        this.fetchAndUpdate(`/v1/tutorials/${slug}`, this.tutorialDetail);
    }
    listUnreadMessages() {
        this.fetchAndUpdate(`/v1/messages?unread_only=true`, this.messageList)
    }
    listEvents() {
        this.fetchAndUpdate('/v1/events', this.eventList);
    }
    listDevices() {
        this.fetchAllPages('/v1/devices', this.deviceList, (acc, extra) => ({
            ...extra,
            devices: [...acc.devices, ...extra.devices],
        })
        );

    }
    listBlueprints() {
        this.fetchAndUpdate('/v1/blueprints', this.blueprintList);
    }
    loadAccount(token: string) {
        localStorage.setItem('token', token);
        this.token.set(token);
        this.auth.token.set(token);
        this.fetchAndUpdate(`/v1/accounts/me`, this.accountData);
    }
    deviceCommand(id: string, command: DeviceCommand) {
        this.postAndUpdate(
            `/v1/devices/${id}`,
            { "command": command },
            this.commandResult);
    }
    markMessagesRead(ml: MessageList) {
        const ids: number[] = [];
        for (const m of ml.messages) {
            ids.push(m.id);
        }
        this.postAndUpdate(
            `/v1/messages/read`,
            { "ids": ids },
            this.markReadResult);
    }

    startMining(id: string, resource: string, site: string) {
        this.postAndUpdate(`/v1/devices/${id}`, {
            command: "start_mining",
            resource_type: resource,
            ...(site && { target: site }),
        }, this.startMiningResult);
    }
    startPrinting(replicant: string, device_type: string) {
        this.postAndUpdate(`/v1/replicants/${replicant}/print`, {
            device_type: device_type,
        }, this.startPrintingResult);
    }
    enqueuePrint(id: string, device_type: string, quantity: number) {
        this.postAndUpdate(`/v1/devices/${id}`, {
            command: "enqueue_print",
            device_type: device_type,
            quantity: quantity,
        }, this.startPrintingResult);
    }
    scan(id: string) {
        this.postAndUpdate(
            `/v1/replicants/${id}/scan`,
            null,
            this.replicantScanResult);
    }
    sendDrone(drone: string, loc: string) {
        this.postAndUpdate(
            `/v1/devices/${drone}`,
            { "command": "travel", "destination": loc },
            this.sendDroneResult);
    }
    patrol(drone: string) {
        this.postAndUpdate(
            `/v1/devices/${drone}`,
            { "command": "set_directive", "directive": "patrol" },
            this.patrolResult);
    }

    depositResources(drone: string, resource: string, amount: string) {
        this.postAndUpdate(`/v1/devices/${drone}`,
            {
                command: "deposit_resources",
                resources: { [resource]: Number(amount) },
            },
            this.emptyTransportResult);
    }
    adoptDrones(controller: string, drones: string[]) {
        this.postAndUpdate(`/v1/devices/${controller}`,
            { command: "adopt", devices: drones },
            this.adoptDronesResult);
    }
    releaseDrones(controller: string, drones: string[]) {
        this.postAndUpdate(`/v1/devices/${controller}`,
            { command: "release", devices: drones },
            this.releaseDronesResult);
    }
    completeEvent(location: string, eventDesignation: string) {
        this.postAndUpdate(`/v1/locations/${location}/events/${eventDesignation}`,
            {},
            this.completeEventResult);
    }
    setDeliveryDirective(controller: string, src: string, dst: string, requirement: Record<string, number>) {
        this.postAndUpdate(`/v1/devices/${controller}`,
            {
                command: "set_directive",
                directive: "delivery",
                configuration: {
                    route: {
                        collect: src,
                        deliver: dst,
                    },
                    requirement: requirement,
                }
            },
            this.setDeliveryDirectiveResult);
    }
    setDirective(controller: string, directive: string, configuration: any) {
        this.postAndUpdate(`/v1/devices/${controller}`,
            {
                command: "set_directive",
                directive: directive,
                configuration: configuration,
            },
            this.setDirectiveResult);
    }
    attach(attacher: string, attached: string) {
        this.postAndUpdate(`/v1/devices/${attacher}`,
            {
                command: "attach",
                device: attached,
            },
            this.commandResult);
    }
    detach(attacher: string, attached: string) {
        this.postAndUpdate(`/v1/devices/${attacher}`,
            {
                command: "detach",
                device: attached,
            },
            this.commandResult);
    }
    scanDrone(drone: string) {
        this.postAndUpdate(
            `/v1/devices/${drone}`,
            { "command": "scan" },
            this.scanDroneResult);
    }
    resources = ["carbon", "conductive", "rares", "silicates", "structural", "volatiles"];

    surgeDestinations = computed<string[] | null>(() => {
        const locations = this.locationList();
        if (!locations) {
            return null;
        }
        return Object.entries(locations.locations).map(
            ([loc, summary]) => loc
        );
    });
    scanDestinations = computed<string[] | null>(() => {
        const scan = this.replicantScanResult();
        if (!scan) {
            return null;
        }
        const reply: string[] = [];
        for (const p of scan.planets) {
            reply.push(p.designation);
        }
        if (scan.asteroid_belt.present) {
            for (const b of scan.asteroid_belt.belts) {
                reply.push(b.designation);
            }
        }
        return reply;
    });
    allDestinations = computed<string[] | null>(() =>
        [...new Set([
            ...(this.surgeDestinations() ?? []),
            ...(this.scanDestinations() ?? [])
        ])].sort()
    );
}

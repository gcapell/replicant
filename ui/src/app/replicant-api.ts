import { Service, signal, inject, WritableSignal, computed } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import {
    AccountData, ScanDroneResult, DeactivateDroneResult,
    DeviceList, Replicant, Location, MessageList,
    SearchResult, Tutorials, TutorialDetail, SendDroneResult,
    StartMiningResult, ReplicantScanResult, BlueprintsList,
    StartPrintingResult, FillTransportResult, EmptyTransportResult, AdoptDronesResult,
    SetDeliveryDirectiveResult, CompleteEventResult, SetDirectiveResult, ReleaseDronesResult,
    PatrolResult, DeployResult, MarkReadResult, LocationList, StowResult,
} from './model';
import { EventList } from './eventmodel';

@Service()
export class ReplicantApi {
    readonly token = signal(localStorage.getItem('token') ?? '');
    protected readonly http = inject(HttpClient);

    readonly accountData = signal<AccountData | null>(null);
    readonly replicant = signal<Replicant | null>(null);
    readonly location = signal<Location | null>(null);
    readonly errorMessage = signal<unknown>(null);
    readonly searchResult = signal<SearchResult | null>(null);
    readonly tutorialList = signal<Tutorials | null>(null);
    readonly tutorialDetail = signal<TutorialDetail | null>(null);
    readonly messageList = signal<MessageList | null>(null);
    readonly eventList = signal<EventList | null>(null);
    readonly startMiningResult = signal<StartMiningResult | null>(null);
    readonly replicantScanResult = signal<ReplicantScanResult | null>(null);
    readonly blueprintList = signal<BlueprintsList | null>(null);
    readonly deviceList = signal<DeviceList | null>(null);
    readonly sendDroneResult = signal<SendDroneResult | null>(null);
    readonly deactivateDroneResult = signal<DeactivateDroneResult | null>(null);
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
    readonly deployResult = signal<DeployResult | null>(null);
        readonly stowResult = signal<StowResult | null>(null);
    readonly markReadResult = signal<MarkReadResult | null>(null);
    readonly locationList = signal<LocationList | null>(null);


    private fetchAndUpdate<T>(path: string, target: WritableSignal<T | null>) {
        console.log("GET", path);
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
            headers: {
                'Authorization': `Bearer ${this.token()}`,
            },
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
    salvageSites = computed<string[]>(() =>
        Object.entries(this.locationList()?.locations ?? {})
            .filter(([, meta]) => meta.resource_sites)
            .map(([loc,]) => loc)
    )


    locations() {
        this.fetchAndUpdate(`/v1/locations`, this.locationList);
        const locs = this.locationList();
        if (!locs) {
            return
        }

    }

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
        this.fetchAndUpdate(`/v1/accounts/me`, this.accountData);
    }
    search(id: string) {
        this.postAndUpdate(
            `/v1/devices/${id}`,
            { "command": "search" },
            this.searchResult);
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
    scan(id: string) {
        this.postAndUpdate(
            `/v1/replicants/${id}/scan`,
            null,
            this.replicantScanResult);
    }
    deactivateDrone(id: string) {
        console.log("deactivate");
        this.postAndUpdate(
            `/v1/devices/${id}`,
            { "command": "deactivate" },
            this.deactivateDroneResult);
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
    deploy(drone: string) {
        this.postAndUpdate(
            `/v1/devices/${drone}`,
            { "command": "deploy" },
            this.deployResult);
    }
    stow(drone: string) {
        this.postAndUpdate(
            `/v1/devices/${drone}`,
            { "command": "stow" },
            this.stowResult);
    }
    fillTransport(drone: string, resource: string) {
        this.postAndUpdate(`/v1/devices/${drone}`,
            {
                command: "collect_resources",
                resources: { [resource]: 20 },
            },
            this.fillTransportResult);
    }
    emptyTransport(drone: string, resource: string) {
        this.postAndUpdate(`/v1/devices/${drone}`,
            {
                command: "deposit_resources",
                resources: { [resource]: 20 },
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
    scanDrone(drone: string) {
        this.postAndUpdate(
            `/v1/devices/${drone}`,
            { "command": "scan" },
            this.scanDroneResult);
    }
    resources = ["carbon", "conductive", "rares", "silicates", "structural", "volatiles"];

    destinations = computed<string[] | null>(() => {
        const scan = this.replicantScanResult();
        if (!scan) {
            return null;
        }
        const reply: string[] = [];
        for (const p of scan.planets) {
            reply.push(p.designation);
        }
        for (const b of scan.asteroid_belt.belts) {
            reply.push(b.designation);
        }
        return reply;
    });
}

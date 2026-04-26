export namespace audit {
	
	export class Entry {
	    // Go type: time
	    time: any;
	    kind: string;
	    reason?: string;
	    remote?: string;
	    peer?: string;
	    detail?: string;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = this.convertValues(source["time"], null);
	        this.kind = source["kind"];
	        this.reason = source["reason"];
	        this.remote = source["remote"];
	        this.peer = source["peer"];
	        this.detail = source["detail"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace main {
	
	export class InterfaceView {
	    name: string;
	    active: boolean;
	    addresses: string[];
	
	    static createFrom(source: any = {}) {
	        return new InterfaceView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.active = source["active"];
	        this.addresses = source["addresses"];
	    }
	}
	export class PeerView {
	    address: string;
	    port: number;
	    signature: string;
	    v2Capable: boolean;
	    fingerprint?: string;
	    paired: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PeerView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.address = source["address"];
	        this.port = source["port"];
	        this.signature = source["signature"];
	        this.v2Capable = source["v2Capable"];
	        this.fingerprint = source["fingerprint"];
	        this.paired = source["paired"];
	    }
	}

}

export namespace settings {
	
	export class PinnedPeer {
	    fingerprint: string;
	    ed25519PubHex: string;
	    label?: string;
	    // Go type: time
	    pinnedAt: any;
	    lastSeenAddr?: string;
	    // Go type: time
	    lastSeenAt?: any;
	
	    static createFrom(source: any = {}) {
	        return new PinnedPeer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fingerprint = source["fingerprint"];
	        this.ed25519PubHex = source["ed25519PubHex"];
	        this.label = source["label"];
	        this.pinnedAt = this.convertValues(source["pinnedAt"], null);
	        this.lastSeenAddr = source["lastSeenAddr"];
	        this.lastSeenAt = this.convertValues(source["lastSeenAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}


export namespace main {
	
	export class PeerView {
	    address: string;
	    port: number;
	    signature: string;
	
	    static createFrom(source: any = {}) {
	        return new PeerView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.address = source["address"];
	        this.port = source["port"];
	        this.signature = source["signature"];
	    }
	}

}


export namespace aria2 {
	
	export class ConfItem {
	    key: string;
	    value: string;
	    comment: string;
	
	    static createFrom(source: any = {}) {
	        return new ConfItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.value = source["value"];
	        this.comment = source["comment"];
	    }
	}
	export class ConfSection {
	    name: string;
	    items: ConfItem[];
	
	    static createFrom(source: any = {}) {
	        return new ConfSection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.items = this.convertValues(source["items"], ConfItem);
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
	export class DetectResult {
	    installed: boolean;
	    version: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new DetectResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.installed = source["installed"];
	        this.version = source["version"];
	        this.path = source["path"];
	    }
	}
	export class ReleaseInfo {
	    version: string;
	    prerelease: boolean;
	    publishedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new ReleaseInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.prerelease = source["prerelease"];
	        this.publishedAt = source["publishedAt"];
	    }
	}

}

export namespace config {
	
	export class Settings {
	    aria2Path: string;
	    rpcPort: number;
	    downloadDir: string;
	    autoStartAria2: boolean;
	    autoStartApp: boolean;
	    closeToTray: boolean;
	    historyMax: number;
	    maxConcurrent: number;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.aria2Path = source["aria2Path"];
	        this.rpcPort = source["rpcPort"];
	        this.downloadDir = source["downloadDir"];
	        this.autoStartAria2 = source["autoStartAria2"];
	        this.autoStartApp = source["autoStartApp"];
	        this.closeToTray = source["closeToTray"];
	        this.historyMax = source["historyMax"];
	        this.maxConcurrent = source["maxConcurrent"];
	    }
	}

}

export namespace history {
	
	export class Record {
	    gid: string;
	    name: string;
	    status: string;
	    totalLength: string;
	    completedLength: string;
	    dir: string;
	    // Go type: time
	    completedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Record(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.gid = source["gid"];
	        this.name = source["name"];
	        this.status = source["status"];
	        this.totalLength = source["totalLength"];
	        this.completedLength = source["completedLength"];
	        this.dir = source["dir"];
	        this.completedAt = this.convertValues(source["completedAt"], null);
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
	
	export class AboutInfo {
	    appVersion: string;
	    aria2Version: string;
	    dataDir: string;
	    cpuCores: number;
	
	    static createFrom(source: any = {}) {
	        return new AboutInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.appVersion = source["appVersion"];
	        this.aria2Version = source["aria2Version"];
	        this.dataDir = source["dataDir"];
	        this.cpuCores = source["cpuCores"];
	    }
	}
	export class Aria2State {
	    running: boolean;
	    pid: number;
	    port: number;
	    connected: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Aria2State(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.pid = source["pid"];
	        this.port = source["port"];
	        this.connected = source["connected"];
	    }
	}

}

export namespace rpc {
	
	export class BTInfo {
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new BTInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	    }
	}
	export class BitTorrent {
	    info: BTInfo;
	
	    static createFrom(source: any = {}) {
	        return new BitTorrent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.info = this.convertValues(source["info"], BTInfo);
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
	export class URIRef {
	    uri: string;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new URIRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.uri = source["uri"];
	        this.status = source["status"];
	    }
	}
	export class File {
	    index: string;
	    path: string;
	    length: string;
	    completedLength: string;
	    selected: string;
	    uris: URIRef[];
	
	    static createFrom(source: any = {}) {
	        return new File(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.path = source["path"];
	        this.length = source["length"];
	        this.completedLength = source["completedLength"];
	        this.selected = source["selected"];
	        this.uris = this.convertValues(source["uris"], URIRef);
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
	export class GlobalStat {
	    downloadSpeed: string;
	    uploadSpeed: string;
	    numActive: string;
	    numWaiting: string;
	    numStopped: string;
	    numStoppedTotal: string;
	
	    static createFrom(source: any = {}) {
	        return new GlobalStat(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.downloadSpeed = source["downloadSpeed"];
	        this.uploadSpeed = source["uploadSpeed"];
	        this.numActive = source["numActive"];
	        this.numWaiting = source["numWaiting"];
	        this.numStopped = source["numStopped"];
	        this.numStoppedTotal = source["numStoppedTotal"];
	    }
	}
	export class Task {
	    gid: string;
	    status: string;
	    totalLength: string;
	    completedLength: string;
	    uploadLength: string;
	    downloadSpeed: string;
	    uploadSpeed: string;
	    connections: string;
	    dir: string;
	    files: File[];
	    bittorrent: BitTorrent;
	    errorCode: string;
	    errorMessage: string;
	    infoHash: string;
	    numPieces: string;
	    pieceLength: string;
	    following: string;
	    followedBy: string[];
	
	    static createFrom(source: any = {}) {
	        return new Task(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.gid = source["gid"];
	        this.status = source["status"];
	        this.totalLength = source["totalLength"];
	        this.completedLength = source["completedLength"];
	        this.uploadLength = source["uploadLength"];
	        this.downloadSpeed = source["downloadSpeed"];
	        this.uploadSpeed = source["uploadSpeed"];
	        this.connections = source["connections"];
	        this.dir = source["dir"];
	        this.files = this.convertValues(source["files"], File);
	        this.bittorrent = this.convertValues(source["bittorrent"], BitTorrent);
	        this.errorCode = source["errorCode"];
	        this.errorMessage = source["errorMessage"];
	        this.infoHash = source["infoHash"];
	        this.numPieces = source["numPieces"];
	        this.pieceLength = source["pieceLength"];
	        this.following = source["following"];
	        this.followedBy = source["followedBy"];
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


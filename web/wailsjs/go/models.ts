export namespace db {
	
	export class Backup {
	    id: number[];
	    server_id: number[];
	    file_path: string;
	    size_bytes: number;
	    sha256: string;
	    status: string;
	    // Go type: time
	    created_at: any;
	
	    static createFrom(source: any = {}) {
	        return new Backup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.server_id = source["server_id"];
	        this.file_path = source["file_path"];
	        this.size_bytes = source["size_bytes"];
	        this.sha256 = source["sha256"];
	        this.status = source["status"];
	        this.created_at = this.convertValues(source["created_at"], null);
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
	export class PlayitAccount {
	    id: number[];
	    name: string;
	    status: string;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new PlayitAccount(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.status = source["status"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
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
	export class Server {
	    id: number[];
	    name: string;
	    server_type: string;
	    version: string;
	    loader_version?: string;
	    image: string;
	    memory_mb: number;
	    cpu_cores: number;
	    volume_name: string;
	    state: string;
	    difficulty: string;
	    online_mode: boolean;
	    motd: string;
	    backup_enabled: boolean;
	    backup_interval_minutes: number;
	    backup_keep: number;
	    // Go type: time
	    backup_last_run?: any;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new Server(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.server_type = source["server_type"];
	        this.version = source["version"];
	        this.loader_version = source["loader_version"];
	        this.image = source["image"];
	        this.memory_mb = source["memory_mb"];
	        this.cpu_cores = source["cpu_cores"];
	        this.volume_name = source["volume_name"];
	        this.state = source["state"];
	        this.difficulty = source["difficulty"];
	        this.online_mode = source["online_mode"];
	        this.motd = source["motd"];
	        this.backup_enabled = source["backup_enabled"];
	        this.backup_interval_minutes = source["backup_interval_minutes"];
	        this.backup_keep = source["backup_keep"];
	        this.backup_last_run = this.convertValues(source["backup_last_run"], null);
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
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
	export class ServerMetric {
	    id: number[];
	    server_id: number[];
	    cpu_pct: number;
	    mem_bytes: number;
	    player_count: number;
	    max_players: number;
	    latency_ms?: number;
	    storage_bytes: number;
	    // Go type: time
	    recorded_at: any;
	
	    static createFrom(source: any = {}) {
	        return new ServerMetric(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.server_id = source["server_id"];
	        this.cpu_pct = source["cpu_pct"];
	        this.mem_bytes = source["mem_bytes"];
	        this.player_count = source["player_count"];
	        this.max_players = source["max_players"];
	        this.latency_ms = source["latency_ms"];
	        this.storage_bytes = source["storage_bytes"];
	        this.recorded_at = this.convertValues(source["recorded_at"], null);
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
	export class ServerTunnel {
	    id: number[];
	    server_id: number[];
	    playit_account_id?: number[];
	    playit_tunnel_id?: number[];
	    sidecar_container_id?: string;
	    public_address?: string;
	    proto: string;
	    status: string;
	    active: boolean;
	    // Go type: time
	    created_at: any;
	
	    static createFrom(source: any = {}) {
	        return new ServerTunnel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.server_id = source["server_id"];
	        this.playit_account_id = source["playit_account_id"];
	        this.playit_tunnel_id = source["playit_tunnel_id"];
	        this.sidecar_container_id = source["sidecar_container_id"];
	        this.public_address = source["public_address"];
	        this.proto = source["proto"];
	        this.status = source["status"];
	        this.active = source["active"];
	        this.created_at = this.convertValues(source["created_at"], null);
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


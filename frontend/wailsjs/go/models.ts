export namespace main {
	
	export class Server {
	    name: string;
	    group: string;
	    host: string;
	    user: string;
	    bastion?: string;
	
	    static createFrom(source: any = {}) {
	        return new Server(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.group = source["group"];
	        this.host = source["host"];
	        this.user = source["user"];
	        this.bastion = source["bastion"];
	    }
	}

}


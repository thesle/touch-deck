export namespace main {
	
	export class ButtonConfig {
	    id: string;
	    label: string;
	    command: string;
	    bgImage: string;
	    bgColor: string;
	    fontColor: string;
	    fontSize: number;
	    order: number;
	
	    static createFrom(source: any = {}) {
	        return new ButtonConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.command = source["command"];
	        this.bgImage = source["bgImage"];
	        this.bgColor = source["bgColor"];
	        this.fontColor = source["fontColor"];
	        this.fontSize = source["fontSize"];
	        this.order = source["order"];
	    }
	}
	export class PageConfig {
	    pageIndex: number;
	    buttons: ButtonConfig[];
	
	    static createFrom(source: any = {}) {
	        return new PageConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pageIndex = source["pageIndex"];
	        this.buttons = this.convertValues(source["buttons"], ButtonConfig);
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
	export class Config {
	    rows: number;
	    cols: number;
	    pages: PageConfig[];
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rows = source["rows"];
	        this.cols = source["cols"];
	        this.pages = this.convertValues(source["pages"], PageConfig);
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


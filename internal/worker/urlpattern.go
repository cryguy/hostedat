package worker

import (
	"fmt"

	v8 "github.com/tommie/v8go"
)

const urlPatternJS = `
(function() {

class URLPattern {
	constructor(input, baseURL) {
		if (typeof input === 'string') {
			if (baseURL) {
				var base = new URL(baseURL);
				this._protocol = base.protocol.replace(':', '');
				this._hostname = base.hostname;
				this._port = base.port;
				this._pathname = input;
				this._search = '';
				this._hash = '';
			} else {
				this._parseFullPattern(input);
			}
		} else if (input && typeof input === 'object') {
			this._protocol = input.protocol || '*';
			this._hostname = input.hostname || '*';
			this._port = input.port || '';
			this._pathname = input.pathname || '*';
			this._search = input.search || '*';
			this._hash = input.hash || '*';
		} else {
			throw new TypeError('Invalid URLPattern input');
		}

		this._pathnameRegex = this._compilePattern(this._pathname || '*');
		this._protocolRegex = this._compilePattern(this._protocol || '*');
		this._hostnameRegex = this._compilePattern(this._hostname || '*');
		this._searchRegex = this._compilePattern(this._search || '*');
		this._hashRegex = this._compilePattern(this._hash || '*');
	}

	_parseFullPattern(pattern) {
		try {
			var match = pattern.match(/^([a-z]+):\/\/([^/:]+)(?::(\d+))?(\/[^?#]*)?(\?[^#]*)?(#.*)?$/);
			if (match) {
				this._protocol = match[1] || '*';
				this._hostname = match[2] || '*';
				this._port = match[3] || '';
				this._pathname = match[4] || '/';
				this._search = match[5] ? match[5].slice(1) : '*';
				this._hash = match[6] ? match[6].slice(1) : '*';
				return;
			}
		} catch(e) {}

		this._protocol = '*';
		this._hostname = '*';
		this._port = '';
		this._pathname = pattern;
		this._search = '*';
		this._hash = '*';
	}

	_compilePattern(pattern) {
		if (pattern === '*') return { regex: /^.*$/, groups: [] };

		var groups = [];
		var regexStr = '^';
		var i = 0;

		while (i < pattern.length) {
			if (pattern[i] === ':') {
				var name = '';
				i++;
				while (i < pattern.length && /[a-zA-Z0-9_]/.test(pattern[i])) {
					name += pattern[i];
					i++;
				}
				groups.push(name);
				regexStr += '([^/]+)';
			} else if (pattern[i] === '*') {
				groups.push('0');
				regexStr += '(.*)';
				i++;
			} else {
				regexStr += pattern[i].replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
				i++;
			}
		}

		regexStr += '$';
		return { regex: new RegExp(regexStr), groups: groups };
	}

	_matchComponent(compiled, value) {
		var match = compiled.regex.exec(value || '');
		if (!match) return null;

		var groups = {};
		for (var i = 0; i < compiled.groups.length; i++) {
			groups[compiled.groups[i]] = match[i + 1] || '';
		}
		return { input: value || '', groups: groups };
	}

	test(input, baseURL) {
		return this.exec(input, baseURL) !== null;
	}

	exec(input, baseURL) {
		var url;
		if (typeof input === 'string') {
			try {
				url = new URL(input, baseURL);
			} catch(e) {
				return null;
			}
		} else if (input instanceof URL) {
			url = input;
		} else if (input && typeof input === 'object') {
			var protocol = input.protocol || 'https:';
			var hostname = input.hostname || 'localhost';
			var port = input.port || '';
			var pathname = input.pathname || '/';
			var search = input.search || '';
			var hash = input.hash || '';
			try {
				url = new URL(protocol + '//' + hostname + (port ? ':' + port : '') + pathname + search + hash);
			} catch(e) {
				return null;
			}
		} else {
			return null;
		}

		var protocol = this._matchComponent(this._protocolRegex, url.protocol.replace(':', ''));
		if (!protocol) return null;

		var hostname = this._matchComponent(this._hostnameRegex, url.hostname);
		if (!hostname) return null;

		var pathname = this._matchComponent(this._pathnameRegex, url.pathname);
		if (!pathname) return null;

		var search = this._matchComponent(this._searchRegex, url.search.replace(/^\?/, ''));
		if (!search) return null;

		var hash = this._matchComponent(this._hashRegex, url.hash.replace(/^#/, ''));
		if (!hash) return null;

		return {
			inputs: [typeof input === 'string' ? input : url.href],
			protocol: protocol,
			hostname: hostname,
			pathname: pathname,
			search: search,
			hash: hash,
			port: { input: url.port, groups: {} },
		};
	}

	get protocol() { return this._protocol; }
	get hostname() { return this._hostname; }
	get port() { return this._port; }
	get pathname() { return this._pathname; }
	get search() { return this._search; }
	get hash() { return this._hash; }
}

globalThis.URLPattern = URLPattern;

})();
`

func setupURLPattern(iso *v8.Isolate, ctx *v8.Context, el *eventLoop) error {
	if _, err := ctx.RunScript(urlPatternJS, "urlpattern.js"); err != nil {
		return fmt.Errorf("evaluating urlpattern.js: %w", err)
	}
	return nil
}

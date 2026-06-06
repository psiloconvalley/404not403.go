// ── 404NOT403 · Header Inspector ─────────────────────────────────────────────
// All dynamic data is set via textContent — never innerHTML.
// XSS safe by design. No exceptions.

const INITIAL_HEADERS_SHOWN = 6;

async function inspectURL() {
    const input   = document.getElementById('inspector-url');
    const btn     = document.getElementById('inspector-btn');
    const result  = document.getElementById('inspector-result');

    const raw = input.value.trim();

    // ── Client-side validation ────────────────────────────────────────────
    if (!raw) {
        showInspectorError(result, 'No URL provided.');
        return;
    }

    // Auto-prepend https:// if no scheme provided
    let url = raw;
    if (!url.startsWith('http://') && !url.startsWith('https://')) {
        url = 'https://' + url.replace(/^www\./, '');
    }

    // ── Loading state ─────────────────────────────────────────────────────
    btn.textContent = 'SCANNING';
    btn.classList.add('scanning');
    input.disabled = true;
    showInspectorLoading(result);

    try {
        const response = await fetch('/api/scan', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ url: url }),
        });

        const data = await response.json();

        if (data.error && data.status_code === 0) {
            showInspectorError(result, data.error);
            return;
        }

        renderScanResult(result, data);
        loadScanHistory();

    } catch (err) {
        showInspectorError(result, 'Request failed. Check your connection.');
    } finally {
        btn.textContent = 'INSPECT';
        btn.classList.remove('scanning');
        input.disabled = false;
    }
}

// ── Render: loading state ─────────────────────────────────────────────────────
function showInspectorLoading(container) {
    container.innerHTML = '';
    const card = document.createElement('div');
    card.className = 'result-placeholder';
    const span = document.createElement('span');
    span.textContent = 'Scanning...';
    card.appendChild(span);
    container.appendChild(card);
}

// ── Render: error state ───────────────────────────────────────────────────────
function showInspectorError(container, message) {
    container.innerHTML = '';

    const card = document.createElement('div');
    card.className = 'scan-error';

    const title = document.createElement('div');
    title.className = 'scan-error-title';
    title.textContent = 'SCAN FAILED';

    const msg = document.createElement('div');
    msg.className = 'scan-error-msg';
    msg.textContent = message;

    card.appendChild(title);
    card.appendChild(msg);
    container.appendChild(card);
}

// ── Render: full scan result ──────────────────────────────────────────────────
function renderScanResult(container, data) {
    container.innerHTML = '';

    const card = document.createElement('div');
    card.className = 'scan-card';

    // ── Verdict bar ───────────────────────────────────────────────────────
    const verdict = document.createElement('div');
    verdict.className = 'scan-verdict ' + statusClass(data.status_code);

    const verdictLeft = document.createElement('div');
    verdictLeft.className = 'verdict-left';

    const verdictStatus = document.createElement('div');
    verdictStatus.className = 'verdict-status';

    const codeEl = document.createElement('span');
    codeEl.className = 'status-code';
    codeEl.textContent = data.status_code || '---';

    const textEl = document.createElement('span');
    textEl.className = 'status-text';
    textEl.textContent = statusLabel(data.status_code);

    verdictStatus.appendChild(codeEl);
    verdictStatus.appendChild(textEl);

    // Summary sentence
    const summary = document.createElement('div');
    summary.className = 'verdict-summary';
    summary.textContent = statusSummary(data.status_code, data);

    verdictLeft.appendChild(verdictStatus);
    verdictLeft.appendChild(summary);

    const meta = document.createElement('div');
    meta.className = 'verdict-meta';

    const duration = document.createElement('span');
    duration.textContent = data.duration_ms + 'ms';

    const size = document.createElement('span');
    size.textContent = formatBytes(data.body_size);

    meta.appendChild(duration);
    meta.appendChild(size);

    verdict.appendChild(verdictLeft);
    verdict.appendChild(meta);

    // ── Evidence section with annotations ─────────────────────────────────
    const evidence = document.createElement('div');
    evidence.className = 'scan-evidence';

    const fields = [
        {
            label: 'SERVER',
            value: data.server,
            cls:   '',
            note:  annotateServer(data.server),
        },
        {
            label: 'CDN',
            value: data.cdn,
            cls:   '',
            note:  annotateCDN(data.cdn),
        },
        {
            label: 'WAF',
            value: data.waf,
            cls:   data.waf ? 'warn' : '',
            note:  annotateWAF(data.waf, data.status_code),
        },
        {
            label: 'TLS',
            value: tlsValue(data),
            cls:   tlsClass(data),
            note:  annotateTLS(data),
        },
        {
            label: 'HASH',
            value: data.body_hash ? data.body_hash.slice(0, 16) + '...' : '',
            cls:   '',
            note:  'SHA256 fingerprint of the response body. If this changes on a future scan, the content changed.',
        },
        {
            label: 'REGION',
            value: data.region,
            cls:   '',
            note:  'The geographic region this scan was performed from. Different regions may receive different responses.',
        },
    ];

    fields.forEach(function(f) {
        const row = document.createElement('div');
        row.className = 'evidence-row';

        const label = document.createElement('div');
        label.className = 'evidence-label';
        label.textContent = f.label;

        const content = document.createElement('div');
        content.className = 'evidence-content';

        const value = document.createElement('div');
        value.className = 'evidence-value' + (f.cls ? ' ' + f.cls : '') + (!f.value ? ' empty' : '');
        value.textContent = f.value || '—';

        content.appendChild(value);

        if (f.note) {
            const note = document.createElement('div');
            note.className = 'evidence-annotation';
            note.textContent = f.note;
            content.appendChild(note);
        }

        row.appendChild(label);
        row.appendChild(content);
        evidence.appendChild(row);
    });

    // ── Headers section ───────────────────────────────────────────────────
    const headersSection = document.createElement('div');
    headersSection.className = 'scan-headers';

    const headersTitle = document.createElement('div');
    headersTitle.className = 'headers-title';

    const headersTitleText = document.createElement('span');
    headersTitleText.textContent = 'RESPONSE HEADERS';

    const headersCount = document.createElement('span');
    headersCount.className = 'headers-count';

    const headers = data.headers || {};
    const headerKeys = Object.keys(headers).sort();
    headersCount.textContent = headerKeys.length + ' found';

    headersTitle.appendChild(headersTitleText);
    headersTitle.appendChild(headersCount);

    const headersGrid = document.createElement('div');
    headersGrid.className = 'headers-grid';

    headerKeys.slice(0, INITIAL_HEADERS_SHOWN).forEach(function(key) {
        appendHeaderRow(headersGrid, key, headers[key]);
    });

    const hiddenHeaders = document.createElement('div');
    hiddenHeaders.className = 'headers-grid';
    hiddenHeaders.style.display = 'none';
    hiddenHeaders.style.marginTop = '0.3rem';

    headerKeys.slice(INITIAL_HEADERS_SHOWN).forEach(function(key) {
        appendHeaderRow(hiddenHeaders, key, headers[key]);
    });

    headersSection.appendChild(headersTitle);
    headersSection.appendChild(headersGrid);
    headersSection.appendChild(hiddenHeaders);

    if (headerKeys.length > INITIAL_HEADERS_SHOWN) {
        const toggle = document.createElement('button');
        toggle.className = 'headers-toggle';
        toggle.textContent = 'SHOW ALL ' + headerKeys.length + ' HEADERS';

        let expanded = false;
        toggle.addEventListener('click', function() {
            expanded = !expanded;
            hiddenHeaders.style.display = expanded ? 'grid' : 'none';
            toggle.textContent = expanded
                ? 'SHOW LESS'
                : 'SHOW ALL ' + headerKeys.length + ' HEADERS';
        });

        headersSection.appendChild(toggle);
    }

    // ── Assemble card ─────────────────────────────────────────────────────
    card.appendChild(verdict);
    card.appendChild(evidence);
    card.appendChild(headersSection);
    container.appendChild(card);
}

// ── DOM helper: append one header row ────────────────────────────────────────
function appendHeaderRow(grid, key, value) {
    const k = document.createElement('div');
    k.className = 'header-key';
    k.textContent = key;

    const v = document.createElement('div');
    v.className = 'header-val';
    v.textContent = value;

    grid.appendChild(k);
    grid.appendChild(v);
}

// ── Status helpers ────────────────────────────────────────────────────────────
function statusClass(code) {
    if (!code) return '';
    if (code >= 200 && code < 300) return 'status-2xx';
    if (code >= 300 && code < 400) return 'status-3xx';
    if (code >= 400 && code < 500) return 'status-4xx';
    if (code >= 500)               return 'status-5xx';
    return '';
}

function statusLabel(code) {
    var labels = {
        200: 'OK',
        201: 'CREATED',
        204: 'NO CONTENT',
        301: 'MOVED PERMANENTLY',
        302: 'FOUND',
        304: 'NOT MODIFIED',
        400: 'BAD REQUEST',
        401: 'UNAUTHORIZED',
        403: 'FORBIDDEN',
        404: 'NOT FOUND',
        405: 'METHOD NOT ALLOWED',
        429: 'TOO MANY REQUESTS',
        500: 'INTERNAL SERVER ERROR',
        502: 'BAD GATEWAY',
        503: 'SERVICE UNAVAILABLE',
        504: 'GATEWAY TIMEOUT',
    };
    return labels[code] || 'UNKNOWN';
}

// ── Status summary — one human sentence per status code ──────────────────────
function statusSummary(code, data) {
    var summaries = {
        200: 'This URL is live and returned content successfully.',
        201: 'The server created a new resource at this URL.',
        204: 'The server responded successfully but returned no content.',
        301: 'This URL has been permanently moved to a new location.',
        302: 'This URL is temporarily redirecting to another location.',
        304: 'The content has not changed since the last request.',
        400: 'The server rejected this request as invalid.',
        401: 'This URL requires authentication to access.',
        403: 'This URL exists but access was denied.',
        404: 'This URL does not exist on the server.',
        405: 'The request method is not supported for this URL.',
        429: 'Too many requests — the server is rate limiting.',
        500: 'The server encountered an internal error.',
        502: 'The upstream server returned an invalid response.',
        503: 'The server is currently unable to handle this request.',
        504: 'The upstream server did not respond in time.',
    };

    if (data.waf && code === 403) {
        return 'A Web Application Firewall actively blocked this request.';
    }

    return summaries[code] || 'The server returned an unexpected status code.';
}

// ── Annotation generators ────────────────────────────────────────────────────
// Each returns a contextual string based on the actual data.
// These are the "translations" that make raw data meaningful.

function annotateServer(server) {
    if (!server) return 'No server header was returned. The origin server is unknown.';

    var s = server.toLowerCase();
    if (s.indexOf('cloudflare') !== -1)  return 'This site is served through Cloudflare\'s global network.';
    if (s.indexOf('nginx') !== -1)       return 'Running on NGINX — one of the most common open-source web servers.';
    if (s.indexOf('apache') !== -1)      return 'Running on Apache — a widely used open-source web server.';
    if (s.indexOf('gws') !== -1)         return 'Google Web Server — this is Google infrastructure.';
    if (s.indexOf('microsoft') !== -1 || s.indexOf('iis') !== -1)
        return 'Running on Microsoft IIS — typically a Windows server environment.';
    if (s.indexOf('openresty') !== -1)   return 'Running on OpenResty — an NGINX-based platform often used with Lua scripting.';
    if (s.indexOf('gunicorn') !== -1)    return 'Running on Gunicorn — a Python WSGI HTTP server, commonly used with Django or Flask.';

    return 'Server identified as "' + server + '".';
}

function annotateCDN(cdn) {
    if (!cdn) return 'No CDN detected. Content may be served directly from the origin server.';

    var notes = {
        'Cloudflare':        'Content is distributed via Cloudflare CDN. The origin server is hidden behind a proxy.',
        'AWS CloudFront':    'Content is distributed via Amazon CloudFront CDN.',
        'Fastly':            'Content is distributed via Fastly CDN — often used by high-traffic media and tech companies.',
        'Akamai':            'Content is distributed via Akamai — one of the oldest and largest CDN networks.',
        'Vercel':            'Deployed on Vercel\'s edge network — commonly used for Next.js and frontend applications.',
        'Netlify':           'Deployed on Netlify\'s edge network — commonly used for static sites and JAMstack.',
        'Railway':           'Deployed on Railway\'s infrastructure.',
        'Generic CDN Cache': 'A caching layer was detected but the specific CDN provider could not be identified.',
    };

    return notes[cdn] || 'CDN identified as "' + cdn + '".';
}

function annotateWAF(waf, statusCode) {
    if (!waf) {
        if (statusCode === 403) {
            return 'No WAF was detected, but access was still denied. The server itself may be enforcing access rules.';
        }
        return 'No Web Application Firewall was detected on this response.';
    }

    var base = 'A Web Application Firewall is actively inspecting requests to this URL.';

    if (statusCode === 403) {
        base += ' The WAF is likely the reason access was denied — the request may have been flagged as suspicious.';
    }

    return base;
}

function annotateTLS(data) {
    if (!data.tls_issuer) {
        if (data.url && data.url.startsWith('http://')) {
            return 'This URL uses plain HTTP — the connection is not encrypted. Data could be intercepted.';
        }
        return 'No TLS certificate information was returned.';
    }

    var exp = data.tls_expiry ? new Date(data.tls_expiry) : null;
    var note = 'Connection is encrypted. Certificate issued by ' + data.tls_issuer + '.';

    if (exp) {
        var daysLeft = Math.floor((exp - new Date()) / (1000 * 60 * 60 * 24));
        if (daysLeft < 0) {
            note += ' The certificate has EXPIRED — this is a security risk.';
        } else if (daysLeft < 30) {
            note += ' The certificate expires in ' + daysLeft + ' days — renewal is needed soon.';
        } else {
            note += ' Valid for ' + daysLeft + ' more days.';
        }
    }

    return note;
}

// ── TLS helpers ───────────────────────────────────────────────────────────────
function tlsValue(data) {
    if (!data.tls_issuer) return '';
    var val = data.tls_issuer;
    if (data.tls_expiry) {
        var exp = new Date(data.tls_expiry);
        var daysLeft = Math.floor((exp - new Date()) / (1000 * 60 * 60 * 24));
        val += ' · exp ' + exp.toISOString().slice(0, 10);
        if (daysLeft < 0) val += ' · EXPIRED';
        else if (daysLeft < 30) val += ' · EXPIRING SOON';
    }
    return val;
}

function tlsClass(data) {
    if (!data.tls_issuer) return 'empty';
    if (data.tls_expiry) {
        var exp = new Date(data.tls_expiry);
        var daysLeft = Math.floor((exp - new Date()) / (1000 * 60 * 60 * 24));
        if (daysLeft < 0) return 'warn';
        if (daysLeft < 30) return 'warn';
        return 'valid';
    }
    return 'valid';
}

// ── Format helpers ────────────────────────────────────────────────────────────
function formatBytes(bytes) {
    if (!bytes) return '0b';
    if (bytes < 1024)        return bytes + 'b';
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + 'kb';
    return (bytes / (1024 * 1024)).toFixed(1) + 'mb';
}

// ── Enter key support ─────────────────────────────────────────────────────────
document.addEventListener('DOMContentLoaded', function() {
    var input = document.getElementById('inspector-url');
    if (input) {
        input.addEventListener('keydown', function(e) {
            if (e.key === 'Enter') inspectURL();
        });
    }
});

// ── Scan History ──────────────────────────────────────────────────────────────
async function loadScanHistory() {
    var container = document.getElementById('scan-history-list');
    var titleEl = document.getElementById('scan-feed-title');
    if (!container) return;

    // Try private history first (requires auth)
    var scans = null;
    var isPrivate = false;

    try {
        var response = await fetch('/api/scans?limit=8', {
            credentials: 'same-origin'
        });
        if (response.ok) {
            scans = await response.json();
            if (scans && scans.length > 0) {
                isPrivate = true;
            }
        }
    } catch (err) {}

    // Fall back to global feed
    if (!scans || scans.length === 0) {
        try {
            var response = await fetch('/api/feed?limit=8');
            if (response.ok) {
                scans = await response.json();
            }
        } catch (err) {}
    }

    // Update section title
    if (titleEl) {
        titleEl.textContent = isPrivate ? 'YOUR SCANS' : 'GLOBAL ACTIVITY';
    var descEl = document.getElementById("scan-feed-desc");
    if (descEl) {
        descEl.textContent = isPrivate ? "Your private forensic scan history." : "Recent forensic events observed across the platform.";
    }
    }

    container.innerHTML = '';

    if (!scans || scans.length === 0) {
        var empty = document.createElement('div');
        empty.className = 'result-placeholder';
        var span = document.createElement('span');
        span.textContent = 'No scans yet';
        empty.appendChild(span);
        container.appendChild(empty);
        return;
    }

    scans.forEach(function(s) {
        container.appendChild(renderScanHistoryRow(s));
    });
}
function renderScanHistoryRow(s) {
    var row = document.createElement('div');
    row.className = 'scan-history-row';

    // Status dot
    var dot = document.createElement('span');
    dot.className = 'monitor-dot ' + historyDotClass(s.status_code);

    // URL
    var url = document.createElement('span');
    url.className = 'scan-history-url';
    url.textContent = s.url.replace('https://', '').replace('http://', '');

    // Status code
    var status = document.createElement('span');
    status.className = 'scan-history-status ' + historyStatusClass(s.status_code);
    status.textContent = s.status_code ? String(s.status_code) : '---';

    // Duration
    var duration = document.createElement('span');
    duration.className = 'scan-history-meta';
    duration.textContent = s.duration_ms + 'ms';

    // Time ago
    var when = document.createElement('span');
    when.className = 'scan-history-meta';
    when.textContent = timeAgoFromISO(s.created_at);

    // Server
    var server = document.createElement('span');
    server.className = 'scan-history-server';
    server.textContent = s.server || '—';

    row.appendChild(dot);
    row.appendChild(url);
    row.appendChild(status);
    row.appendChild(duration);
    row.appendChild(server);
    row.appendChild(when);

    return row;
}

function historyDotClass(code) {
    if (!code) return 'dot-pending';
    if (code >= 200 && code < 300) return 'dot-ok';
    if (code >= 300 && code < 400) return 'dot-warn';
    if (code >= 400) return 'dot-bad';
    return 'dot-pending';
}

function historyStatusClass(code) {
    if (!code) return '';
    if (code >= 200 && code < 300) return 'status-ok';
    if (code >= 300 && code < 400) return 'status-warn';
    if (code >= 400) return 'status-bad';
    return '';
}

function timeAgoFromISO(iso) {
    var now = new Date();
    var then = new Date(iso);
    var seconds = Math.floor((now - then) / 1000);
    if (seconds < 60)    return seconds + 's ago';
    if (seconds < 3600)  return Math.floor(seconds / 60) + 'm ago';
    if (seconds < 86400) return Math.floor(seconds / 3600) + 'h ago';
    return Math.floor(seconds / 86400) + 'd ago';
}

// Call loadScanHistory after every successful scan
// and on page load
document.addEventListener('DOMContentLoaded', function() {
    loadScanHistory();
});

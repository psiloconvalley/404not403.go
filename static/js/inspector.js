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

    if (!raw.startsWith('http://') && !raw.startsWith('https://')) {
        showInspectorError(result, 'URL must begin with http:// or https://');
        return;
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
            body: JSON.stringify({ url: raw }),
        });

        const data = await response.json();

        if (data.error && data.status_code === 0) {
            showInspectorError(result, data.error);
            return;
        }

        renderScanResult(result, data);

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

    const meta = document.createElement('div');
    meta.className = 'verdict-meta';

    const duration = document.createElement('span');
    duration.textContent = data.duration_ms + 'ms';

    const size = document.createElement('span');
    size.textContent = formatBytes(data.body_size);

    meta.appendChild(duration);
    meta.appendChild(size);

    verdict.appendChild(verdictStatus);
    verdict.appendChild(meta);

    // ── Evidence grid ─────────────────────────────────────────────────────
    const evidence = document.createElement('div');
    evidence.className = 'scan-evidence';

    const fields = [
        { label: 'URL',     value: data.url,        cls: '' },
        { label: 'SERVER',  value: data.server,      cls: '' },
        { label: 'CDN',     value: data.cdn,         cls: '' },
        { label: 'WAF',     value: data.waf,         cls: data.waf ? 'warn' : '' },
        { label: 'TLS',     value: tlsValue(data),   cls: tlsClass(data) },
        { label: 'HASH',    value: data.body_hash ? data.body_hash.slice(0, 16) + '...' : '', cls: '' },
        { label: 'REGION',  value: data.region,      cls: '' },
    ];

    fields.forEach(function(f) {
        const label = document.createElement('div');
        label.className = 'evidence-label';
        label.textContent = f.label;

        const value = document.createElement('div');
        value.className = 'evidence-value' + (f.cls ? ' ' + f.cls : '') + (!f.value ? ' empty' : '');
        value.textContent = f.value || '—';

        evidence.appendChild(label);
        evidence.appendChild(value);
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

    // Render first N headers
    headerKeys.slice(0, INITIAL_HEADERS_SHOWN).forEach(function(key) {
        appendHeaderRow(headersGrid, key, headers[key]);
    });

    // Hidden headers container
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

    // Toggle button — only if there are hidden headers
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
    const labels = {
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

// ── TLS helpers ───────────────────────────────────────────────────────────────
function tlsValue(data) {
    if (!data.tls_issuer) return '';
    let val = data.tls_issuer;
    if (data.tls_expiry) {
        const exp = new Date(data.tls_expiry);
        const now = new Date();
        const daysLeft = Math.floor((exp - now) / (1000 * 60 * 60 * 24));
        val += ' · exp ' + exp.toISOString().slice(0, 10);
        if (daysLeft < 30) val += ' · EXPIRING SOON';
    }
    return val;
}

function tlsClass(data) {
    if (!data.tls_issuer) return 'empty';
    if (data.tls_expiry) {
        const exp = new Date(data.tls_expiry);
        const daysLeft = Math.floor((exp - new Date()) / (1000 * 60 * 60 * 24));
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
    const input = document.getElementById('inspector-url');
    if (input) {
        input.addEventListener('keydown', function(e) {
            if (e.key === 'Enter') inspectURL();
        });
    }
});

// ── 404NOT403 · Status Simulator ─────────────────────────────────────────────
// All dynamic data is set via textContent — never innerHTML.
// XSS safe by design. No exceptions.

async function simulate(code) {
    const responseBox    = document.getElementById('response-box');
    const educationPanel = document.getElementById('education-panel');

    // Reset state
    responseBox.className = 'response-box';
    educationPanel.className = 'education-panel';

    // Loading state — safe DOM construction
    responseBox.innerHTML = '';
    const loadingEl = document.createElement('p');
    loadingEl.className = 'placeholder';
    loadingEl.textContent = 'Sending request...';
    responseBox.appendChild(loadingEl);

    try {
        const response = await fetch('/simulate/' + code);
        const data = await response.json();

        // ── Build response display safely ─────────────────────────────────
        responseBox.innerHTML = '';

        const received = document.createElement('div');
        received.className = 'sim-label';
        received.textContent = 'HTTP Response Received';
        responseBox.appendChild(received);

        const spacer = document.createElement('div');
        spacer.style.height = '0.75rem';
        responseBox.appendChild(spacer);

        const fields = [
            { label: 'Status',  value: String(data.status) },
            { label: 'Error',   value: data.error           },
            { label: 'Message', value: data.message         },
        ];

        fields.forEach(function(f) {
            const row = document.createElement('div');
            row.className = 'sim-row';

            const label = document.createElement('span');
            label.className = 'sim-field-label';
            label.textContent = f.label + ':';

            const value = document.createElement('span');
            value.className = 'sim-field-value';
            value.textContent = ' ' + f.value;

            row.appendChild(label);
            row.appendChild(value);
            responseBox.appendChild(row);
        });

        const spacer2 = document.createElement('div');
        spacer2.style.height = '0.75rem';
        responseBox.appendChild(spacer2);

        const tip = document.createElement('div');
        tip.className = 'sim-tip';
        tip.textContent = data.tip;
        responseBox.appendChild(tip);

        // ── Style and education panel ─────────────────────────────────────
        if (code === 404) {
            responseBox.classList.add('error-404');
            educationPanel.className = 'education-panel panel-404 visible';
            renderEducation(educationPanel, {
                title:   'UNDERSTANDING 404 NOT FOUND',
                color:   'var(--status-block)',
                code:    '404',
                body1:   'A 404 means the server was reached successfully, but the specific resource — page, file, or endpoint — does not exist at that URL.',
                body2:   'Think of it like going to a library and asking for a book that was never written. The library exists. The librarian is there. But the book? Gone.',
                fix:     'Common Fixes: Check the URL for typos. The page may have moved — look for 301 redirects. The resource may have been deleted entirely.',
            });
        }

        if (code === 403) {
            responseBox.classList.add('error-403');
            educationPanel.className = 'education-panel panel-403 visible';
            renderEducation(educationPanel, {
                title:   'UNDERSTANDING 403 FORBIDDEN',
                color:   'var(--status-warn)',
                code:    '403',
                body1:   'A 403 means the server was reached and the resource EXISTS — but you do not have permission to access it.',
                body2:   'Think of it like going to a library and asking for a book locked in the restricted section. The book is there. But you are not on the list.',
                fix:     'Common Fixes: Check your authentication token. You may need to log in. The server may be blocking your IP address or User-Agent string.',
            });
        }

        fetchStats();

    } catch (err) {
        responseBox.innerHTML = '';
        const errEl = document.createElement('p');
        errEl.className = 'sim-error';
        errEl.textContent = 'Request failed. Check your connection.';
        responseBox.appendChild(errEl);
    }
}

// ── Build education panel safely ──────────────────────────────────────────────
function renderEducation(panel, opts) {
    panel.innerHTML = '';

    const title = document.createElement('h3');
    title.textContent = opts.title;
    panel.appendChild(title);

    const p1 = document.createElement('p');
    p1.textContent = opts.body1;
    panel.appendChild(p1);

    const p2 = document.createElement('p');
    p2.style.marginTop = '0.75rem';
    p2.textContent = opts.body2;
    panel.appendChild(p2);

    const fix = document.createElement('p');
    fix.className = 'fix';
    fix.textContent = opts.fix;
    panel.appendChild(fix);
}

// ── Fetch live stats ──────────────────────────────────────────────────────────
async function fetchStats() {
    const statsEl = document.getElementById('live-stats');
    if (!statsEl) return;

    try {
        const response = await fetch('/api/stats');
        const data = await response.json();

        statsEl.innerHTML = '';

        const total = document.createElement('span');
        total.className = 'stat-total';
        total.textContent = data.total + ' events logged';

        const div1 = document.createElement('span');
        div1.className = 'stat-divider';
        div1.textContent = '·';

        const s404 = document.createElement('span');
        s404.className = 'stat-404';
        s404.textContent = data['404s'] + ' not found';

        const div2 = document.createElement('span');
        div2.className = 'stat-divider';
        div2.textContent = '·';

        const s403 = document.createElement('span');
        s403.className = 'stat-403';
        s403.textContent = data['403s'] + ' forbidden';

        statsEl.appendChild(total);
        statsEl.appendChild(div1);
        statsEl.appendChild(s404);
        statsEl.appendChild(div2);
        statsEl.appendChild(s403);

    } catch (err) {
        statsEl.innerHTML = '';
        const unavail = document.createElement('span');
        unavail.className = 'stats-loading';
        unavail.textContent = 'Stats unavailable';
        statsEl.appendChild(unavail);
    }
}

// Load stats on page load
fetchStats();

// ── 404NOT403 · Ghost Link Monitor ───────────────────────────────────────────
// All dynamic data is set via textContent — never innerHTML with external data.
// XSS safe by design. No exceptions.

// ── Create a new monitor ──────────────────────────────────────────────────────
async function createMonitor() {
    var input  = document.getElementById('monitor-url');
    var select = document.getElementById('monitor-interval');
    var btn    = document.getElementById('monitor-btn');

    var raw = input.value.trim();
    if (!raw) return;

    if (!raw.startsWith('http://') && !raw.startsWith('https://')) {
        raw = 'https://' + raw.replace(/^www\./, '');
    }

    btn.textContent = 'SAVING';
    btn.disabled    = true;
    input.disabled  = true;

    try {
        var response = await fetch('/api/monitor', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ url: raw, interval: select.value }),
        });

        var data = await response.json();

        if (data.error) {
            alert('Monitor error: ' + data.error);
            return;
        }

        input.value = '';
        loadMonitors();

    } catch (err) {
        alert('Failed to create monitor.');
    } finally {
        btn.textContent = 'MONITOR';
        btn.disabled    = false;
        input.disabled  = false;
    }
}

// ── Load and render active monitors ──────────────────────────────────────────
async function loadMonitors() {
    var container = document.getElementById('monitor-list');

    try {
        var response = await fetch('/api/monitors');
        var monitors = await response.json();

        container.innerHTML = '';

        if (!monitors || monitors.length === 0) {
            var placeholder = document.createElement('div');
            placeholder.className = 'result-placeholder';
            var span = document.createElement('span');
            span.textContent = 'No monitors active';
            placeholder.appendChild(span);
            container.appendChild(placeholder);
            return;
        }

        var header = document.createElement('div');
        header.className = 'monitor-list-header';

        var title = document.createElement('span');
        title.textContent = 'ACTIVE MONITORS';

        var count = document.createElement('span');
        count.className = 'monitor-count';
        count.textContent = monitors.length + ' active';

        header.appendChild(title);
        header.appendChild(count);
        container.appendChild(header);

        monitors.forEach(function(m) {
            container.appendChild(renderMonitorRow(m));
        });

        loadChanges();

    } catch (err) {
        container.innerHTML = '';
        var errEl = document.createElement('div');
        errEl.className = 'result-placeholder';
        var span = document.createElement('span');
        span.textContent = 'Failed to load monitors';
        errEl.appendChild(span);
        container.appendChild(errEl);
    }
}

// ── Render a single monitor row ───────────────────────────────────────────────
function renderMonitorRow(m) {
    var row = document.createElement('div');
    row.className = 'monitor-row';

    var dot = document.createElement('span');
    dot.className = 'monitor-dot ' + monitorDotClass(m);

    var url = document.createElement('span');
    url.className = 'monitor-url';
    url.textContent = m.url.replace('https://', '').replace('http://', '');

    var status = document.createElement('span');
    status.className = 'monitor-status ' + monitorStatusClass(m);
    status.textContent = m.last_status ? String(m.last_status) : '---';

    var interval = document.createElement('span');
    interval.className = 'monitor-interval';
    interval.textContent = m.check_interval;

    var checked = document.createElement('span');
    checked.className = 'monitor-checked';
    checked.textContent = m.last_checked ? timeAgo(m.last_checked) : 'pending';

    var changes = document.createElement('span');
    changes.className = 'monitor-changes';
    if (m.change_count > 0) {
        changes.textContent = m.change_count + ' change' + (m.change_count > 1 ? 's' : '');
        changes.classList.add('has-changes');
    } else {
        changes.textContent = 'stable';
    }

    // Delete button
    var del = document.createElement('button');
    del.className = 'monitor-delete';
    del.textContent = '×';
    del.addEventListener('click', function() {
        deleteMonitor(m.id, row);
    });

    row.appendChild(dot);
    row.appendChild(url);
    row.appendChild(status);
    row.appendChild(interval);
    row.appendChild(checked);
    row.appendChild(changes);
    row.appendChild(del);

    return row;
}

// ── Delete a monitor ──────────────────────────────────────────────────────────
async function deleteMonitor(id, row) {
    try {
        var response = await fetch('/api/monitor?id=' + encodeURIComponent(id), {
            method: 'DELETE',
        });
        var data = await response.json();
        if (data.status === 'deactivated') {
            row.style.opacity = '0.3';
            row.style.pointerEvents = 'none';
            setTimeout(function() {
                loadMonitors();
            }, 400);
        }
    } catch (err) {
        alert('Failed to delete monitor.');
    }
}

// ── Load and render detected changes ─────────────────────────────────────────
async function loadChanges() {
    var section   = document.getElementById('changes-section');
    var container = document.getElementById('changes-list');

    try {
        var response = await fetch('/api/changes');
        var changes  = await response.json();

        if (!changes || changes.length === 0) {
            section.style.display = 'none';
            return;
        }

        section.style.display = 'block';
        container.innerHTML   = '';

        changes.forEach(function(c) {
            container.appendChild(renderChangeRow(c));
        });

    } catch (err) {
        section.style.display = 'none';
    }
}

// ── Render a single change row ────────────────────────────────────────────────
function renderChangeRow(c) {
    var row = document.createElement('div');
    row.className = 'change-row';

    var url = document.createElement('span');
    url.className = 'change-url';
    url.textContent = c.url.replace('https://', '').replace('http://', '');

    var transition = document.createElement('span');
    transition.className = 'change-transition';
    var oldCode = c.old_status || '---';
    var newCode = c.new_status || '---';
    transition.textContent = oldCode + ' → ' + newCode;

    if (c.new_status >= 400) {
        transition.classList.add('change-bad');
    } else if (c.new_status >= 200 && c.new_status < 300) {
        transition.classList.add('change-good');
    }

    var when = document.createElement('span');
    when.className = 'change-when';
    when.textContent = timeAgo(c.detected_at);

    row.appendChild(url);
    row.appendChild(transition);
    row.appendChild(when);

    return row;
}

// ── Status helpers ────────────────────────────────────────────────────────────
function monitorDotClass(m) {
    if (!m.last_status) return 'dot-pending';
    if (m.last_status >= 200 && m.last_status < 300) return 'dot-ok';
    if (m.last_status >= 300 && m.last_status < 400) return 'dot-warn';
    if (m.last_status >= 400) return 'dot-bad';
    return 'dot-pending';
}

function monitorStatusClass(m) {
    if (!m.last_status) return '';
    if (m.last_status >= 200 && m.last_status < 300) return 'status-ok';
    if (m.last_status >= 300 && m.last_status < 400) return 'status-warn';
    if (m.last_status >= 400) return 'status-bad';
    return '';
}

// ── Time helper ───────────────────────────────────────────────────────────────
function timeAgo(dateStr) {
    var now     = new Date();
    var then    = new Date(dateStr);
    var seconds = Math.floor((now - then) / 1000);

    if (seconds < 60)    return seconds + 's ago';
    if (seconds < 3600)  return Math.floor(seconds / 60) + 'm ago';
    if (seconds < 86400) return Math.floor(seconds / 3600) + 'h ago';
    return Math.floor(seconds / 86400) + 'd ago';
}

// ── Init ──────────────────────────────────────────────────────────────────────
document.addEventListener('DOMContentLoaded', function() {
    loadMonitors();

    var input = document.getElementById('monitor-url');
    if (input) {
        input.addEventListener('keydown', function(e) {
            if (e.key === 'Enter') createMonitor();
        });
    }
});

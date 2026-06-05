// ── 404NOT403 · Ghost Link Monitor ───────────────────────────────────────────
// All dynamic data is set via textContent — never innerHTML with external data.
// XSS safe by design. No exceptions.

// ── Create a new monitor ──────────────────────────────────────────────────────
async function createMonitor() {
    var input = document.getElementById('monitor-url');
    var select = document.getElementById('monitor-interval');
    var btn = document.getElementById('monitor-btn');

    var raw = input.value.trim();
    if (!raw) return;

    // Auto-prepend https://
    if (!raw.startsWith('http://') && !raw.startsWith('https://')) {
        raw = 'https://' + raw.replace(/^www\./, '');
    }

    btn.textContent = 'SAVING';
    btn.disabled = true;
    input.disabled = true;

    try {
        var response = await fetch('/api/monitor', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                url: raw,
                interval: select.value,
            }),
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
        btn.disabled = false;
        input.disabled = false;
    }
}

// ── Load and render active monitors ───────────────────────────────────────────
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

        // Load changes
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

    // Status dot
    var dot = document.createElement('span');
    dot.className = 'monitor-dot ' + monitorDotClass(m);

    // URL
    var url = document.createElement('span');
    url.className = 'monitor-url';
    url.textContent = m.URL.replace('https://', '').replace('http://', '');

    // Status code
    var status = document.createElement('span');
    status.className = 'monitor-status ' + monitorStatusClass(m);
    status.textContent = m.LastStatus ? String(m.LastStatus) : '---';

    // Interval
    var interval = document.createElement('span');
    interval.className = 'monitor-interval';
    interval.textContent = m.CheckInterval;

    // Last checked
    var checked = document.createElement('span');
    checked.className = 'monitor-checked';
    checked.textContent = m.LastChecked ? timeAgo(m.LastChecked) : 'pending';

    // Change count
    var changes = document.createElement('span');
    changes.className = 'monitor-changes';
    if (m.ChangeCount > 0) {
        changes.textContent = m.ChangeCount + ' change' + (m.ChangeCount > 1 ? 's' : '');
        changes.classList.add('has-changes');
    } else {
        changes.textContent = 'stable';
    }

    row.appendChild(dot);
    row.appendChild(url);
    row.appendChild(status);
    row.appendChild(interval);
    row.appendChild(checked);
    row.appendChild(changes);

    return row;
}

// ── Load and render detected changes ──────────────────────────────────────────
async function loadChanges() {
    var section = document.getElementById('changes-section');
    var container = document.getElementById('changes-list');

    try {
        var response = await fetch('/api/changes');
        var changes = await response.json();

        if (!changes || changes.length === 0) {
            section.style.display = 'none';
            return;
        }

        section.style.display = 'block';
        container.innerHTML = '';

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
    url.textContent = c.URL.replace('https://', '').replace('http://', '');

    var transition = document.createElement('span');
    transition.className = 'change-transition';
    var oldCode = c.OldStatus || '---';
    var newCode = c.NewStatus || '---';
    transition.textContent = oldCode + ' → ' + newCode;

    // Color the transition based on the new status
    if (c.NewStatus >= 400) {
        transition.classList.add('change-bad');
    } else if (c.NewStatus >= 200 && c.NewStatus < 300) {
        transition.classList.add('change-good');
    }

    var when = document.createElement('span');
    when.className = 'change-when';
    when.textContent = timeAgo(c.DetectedAt);

    row.appendChild(url);
    row.appendChild(transition);
    row.appendChild(when);

    return row;
}

// ── Helper: dot color class ───────────────────────────────────────────────────
function monitorDotClass(m) {
    if (!m.LastStatus) return 'dot-pending';
    if (m.LastStatus >= 200 && m.LastStatus < 300) return 'dot-ok';
    if (m.LastStatus >= 300 && m.LastStatus < 400) return 'dot-warn';
    if (m.LastStatus >= 400) return 'dot-bad';
    return 'dot-pending';
}

// ── Helper: status text color ─────────────────────────────────────────────────
function monitorStatusClass(m) {
    if (!m.LastStatus) return '';
    if (m.LastStatus >= 200 && m.LastStatus < 300) return 'status-ok';
    if (m.LastStatus >= 300 && m.LastStatus < 400) return 'status-warn';
    if (m.LastStatus >= 400) return 'status-bad';
    return '';
}

// ── Helper: relative time ─────────────────────────────────────────────────────
function timeAgo(dateStr) {
    var now = new Date();
    var then = new Date(dateStr);
    var seconds = Math.floor((now - then) / 1000);

    if (seconds < 60)   return seconds + 's ago';
    if (seconds < 3600)  return Math.floor(seconds / 60) + 'm ago';
    if (seconds < 86400) return Math.floor(seconds / 3600) + 'h ago';
    return Math.floor(seconds / 86400) + 'd ago';
}

// ── Load monitors on page load ────────────────────────────────────────────────
document.addEventListener('DOMContentLoaded', function() {
    loadMonitors();

    // Enter key support
    var input = document.getElementById('monitor-url');
    if (input) {
        input.addEventListener('keydown', function(e) {
            if (e.key === 'Enter') createMonitor();
        });
    }
});

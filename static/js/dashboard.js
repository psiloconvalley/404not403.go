// ── State ──────────────────────────────────────────────────────────────────
let currentOrgID    = null;
let currentOrgName  = null;
let currentTicketID = null;
let currentFilter   = 'all';
let currentQueueID  = null;
let currentOffset   = 0;
let hasMoreTickets  = false;
let currentQueueName = null;
let currentSettingsQueueID = null;
let sidebarData     = null;


// ── CSRF ───────────────────────────────────────────────────────────────────
// Reads the csrf_token cookie set by the server on login.
// Attached to every state-changing fetch request as X-CSRF-Token header.
function getCSRFToken() {
    const match = document.cookie.split(';')
        .map(function(c) { return c.trim(); })
        .find(function(c) { return c.startsWith('csrf_token='); });
    return match ? match.split('=')[1] : '';
}

// csrfHeaders returns headers with CSRF token + Content-Type.
// Use for every POST/DELETE fetch call.
function csrfHeaders() {
    return {
        'Content-Type': 'application/json',
        'X-CSRF-Token': getCSRFToken()
    };
}

// ── Init ───────────────────────────────────────────────────────────────────
async function init() {
    const res = await fetch('/api/auth/me');
    if (!res.ok) { window.location.href = '/login'; return; }
    const user = await res.json();
    document.getElementById('user-handle').textContent = user.handle;
    loadOrgs();
}

// ── Views ──────────────────────────────────────────────────────────────────
function showView(name) {
    ['org','queue','detail','settings-org','settings-account','settings-queue'].forEach(function(v) {
        const el = document.getElementById('view-' + v);
        if (el) el.style.display = 'none';
    });

    const target = document.getElementById('view-' + name);
    if (target) target.style.display = 'block';

    const sidebar = document.getElementById('global-sidebar');
    if (sidebar) sidebar.style.display = (name === 'org') ? 'none' : '';

    if (name === 'settings-org') loadOrgSettings();
    if (name === 'settings-queue') loadQueueSettings();
    if (name === 'settings-account') loadAccountSettings();
}

function showQueue() {
    showView('queue');
    currentTicketID = null;
    loadTickets(currentFilter); showView('queue');
}

// ── Orgs ───────────────────────────────────────────────────────────────────
async function loadOrgs() {
    const res = await fetch('/api/orgs/me');
    if (!res.ok) { showView('org'); return; }
    const data = await res.json();
    const list = document.getElementById('org-list');
    list.innerHTML = '';

    if (!data.orgs || data.orgs.length === 0) {
        list.innerHTML = '<p class="empty">No organizations yet. Create one to get started.</p>';
        showView('org');
        return;
    }

    if (data.orgs.length === 1) {
        selectOrg(data.orgs[0].org_id, data.orgs[0].org_name);
        return;
    }

    data.orgs.forEach(function(m) {
        const div = document.createElement('div');
        div.className = 'org-card';
        div.onclick = function() { selectOrg(m.org_id, m.org_name); };
        const name = document.createElement('span');
        name.className = 'org-name';
        name.textContent = m.org_name;
        div.appendChild(name);
        const role = document.createElement('span');
        role.className = 'org-role';
        role.textContent = m.role;
        div.appendChild(role);
        list.appendChild(div);
    });

    showView('org');
}

function selectOrg(orgID, orgName) {
    currentOrgID   = orgID;
    currentOrgName = orgName;
    document.getElementById('nav-org-name').textContent = orgName;
    document.getElementById('nav-org-name').style.display = 'block';
    showView('queue');
    loadSidebar();
    loadTickets('all');
}

function showCreateOrg() { document.getElementById('create-org-form').style.display = 'flex'; }
function hideCreateOrg()  { document.getElementById('create-org-form').style.display = 'none'; }

async function createOrg() {
    const name = document.getElementById('org-name').value.trim();
    const slug = document.getElementById('org-slug').value.trim();
    if (!name || !slug) return;

    const res = await fetch('/api/orgs', {
        method: 'POST',
        headers: csrfHeaders(),
        body: JSON.stringify({name: name, slug: slug})
    });

    if (res.ok) {
        hideCreateOrg();
        document.getElementById('org-name').value = '';
        document.getElementById('org-slug').value = '';
        loadOrgs();
    } else {
        const data = await res.json();
        showToast('error', data.error || 'Failed to create organization');
    }
}

// ── Sidebar ─────────────────────────────────────────────────────────────────
async function loadSidebar() {
    if (!currentOrgID) return;
    const res = await fetch('/api/orgs/' + currentOrgID + '/queues/sidebar');
    if (!res.ok) return;
    sidebarData = await res.json();
    renderSidebar();
}

function renderSidebar() {
    if (!sidebarData) return;
    const counts = sidebarData.org_counts || {};
    const queues = sidebarData.queues || [];

    // Update counts — single global sidebar
    const totalEl      = document.getElementById('count-total');
    const unassignedEl = document.getElementById('count-unassigned');
    const urgentEl     = document.getElementById('count-urgent');
    if (totalEl)      totalEl.textContent      = counts.total      || 0;
    if (unassignedEl) unassignedEl.textContent = counts.unassigned || 0;
    if (urgentEl)     urgentEl.textContent      = counts.urgent     || 0;

    // Role-based settings visibility
    const role = sidebarData.role || 'viewer';
    const orgSettingsEl = document.getElementById('nav-settings-org');
    if (orgSettingsEl) orgSettingsEl.style.display = (role === 'owner' || role === 'admin') ? '' : 'none';

    // Render queue list — single global sidebar
    const list = document.getElementById('queue-list');
    if (!list) return;
    list.innerHTML = '';
        if (!queues || queues.length === 0) {
            list.innerHTML = '<div class="sidebar-empty">No queues yet</div>';
            return;
        }

        // Group by department
        const grouped = {};
        queues.forEach(function(q) {
            const dept = q.department || 'General';
            if (!grouped[dept]) grouped[dept] = [];
            grouped[dept].push(q);
        });

        Object.keys(grouped).sort().forEach(function(dept) {
            // Department header
            const deptEl = document.createElement('div');
            deptEl.className = 'sidebar-dept';
            deptEl.textContent = dept;
            list.appendChild(deptEl);

            grouped[dept].forEach(function(q) {
                const item = document.createElement('div');
                item.className = 'sidebar-item' +
                    (currentQueueID === q.id ? ' sidebar-item-active' : '');
                item.onclick = function() { selectQueue(q.id, q.name); };

                const nameEl = document.createElement('span');
                nameEl.className = 'sidebar-item-name';
                // Color dot
                const dot = document.createElement('span');
                dot.className = 'sidebar-dot';
                dot.style.background = q.color || '#6366f1';
                nameEl.appendChild(dot);
                nameEl.appendChild(document.createTextNode(q.name));
                item.appendChild(nameEl);

                const countEl = document.createElement('span');
                countEl.className = 'sidebar-count' +
                    (q.urgent_count > 0 ? ' sidebar-count-crit' : '');
                countEl.textContent = q.open_count || 0;
                item.appendChild(countEl);

                // Settings gear for queue — admin/owner only
                const role = (sidebarData && sidebarData.role) || 'viewer';
                if (role === 'owner' || role === 'admin') {
                    const gear = document.createElement('span');
                    gear.className = 'sidebar-gear';
                    gear.innerHTML = icon('settings', 'sm');
                    gear.onclick = function(e) {
                        e.stopPropagation();
                        currentSettingsQueueID = q.id;
                        showView('settings-queue');
                    };
                    item.appendChild(gear);
                }

                list.appendChild(item);
            });
        });
}

// ── Queue Selection ──────────────────────────────────────────────────────────
function selectAllTickets() {
    currentQueueID   = null;
    currentQueueName = null;
    document.getElementById('queue-title').textContent = 'My Requests';
    setSidebarActive('sidebar-all');
    loadTickets(currentFilter); showView('queue');
}

function selectUnassigned() {
    currentQueueID   = null;
    currentQueueName = 'Needs Owner';
    document.getElementById('queue-title').textContent = 'Needs Owner';
    setSidebarActive('sidebar-unassigned'); loadTicketsFiltered({assigned_to: 'null'}); showView('queue');
}

function selectUrgent() {
    currentQueueID   = null;
    currentQueueName = 'Urgent';
    document.getElementById('queue-title').textContent = 'Urgent — P0 / P1';
    setSidebarActive('sidebar-urgent'); loadTicketsFiltered({priority: 'P0'}); showView('queue');
}

function selectQueue(queueID, queueName) {
    currentQueueID   = queueID;
    currentQueueName = queueName;
    document.getElementById('queue-title').textContent = queueName;
    renderSidebar(); // refresh active state
    loadQueueTickets(queueID);
    showView('queue');
}

function setSidebarActive(itemID) {
    document.querySelectorAll('.sidebar-item').forEach(function(el) {
        el.classList.remove('sidebar-item-active');
    });
    const el = document.getElementById(itemID);
    if (el) el.classList.add('sidebar-item-active');
}

async function loadQueueTickets(queueID) {
    if (!currentOrgID) return;
    const res = await fetch('/api/orgs/' + currentOrgID + '/queues/' + queueID + '/tickets');
    if (!res.ok) return;
    const data = await res.json();
    renderTicketList(data.tickets || []);
    document.getElementById('load-more-btn').style.display = 'none';
    hasMoreTickets = false;
}

async function loadTicketsFiltered(params) {
    if (!currentOrgID) return;
    let url = '/api/orgs/' + currentOrgID + '/tickets?';
    Object.keys(params).forEach(function(k) {
        url += k + '=' + params[k] + '&';
    });
    const res = await fetch(url);
    if (!res.ok) return;
    const data = await res.json();
    renderTicketList(data.tickets || []);
    document.getElementById('load-more-btn').style.display = 'none';
    hasMoreTickets = false;
}

// ── Queue CRUD ───────────────────────────────────────────────────────────────
function showCreateQueue() { document.getElementById('create-queue-form').style.display = 'block'; }
function hideCreateQueue()  { document.getElementById('create-queue-form').style.display = 'none'; }

async function createQueue() {
    if (!currentOrgID) return;
    const name  = document.getElementById('queue-name').value.trim();
    const dept  = document.getElementById('queue-dept').value.trim();
    const color = document.getElementById('queue-color').value.trim();
    if (!name) { showToast('warn', 'Queue name is required'); return; }

    const prefix = document.getElementById('queue-prefix').value.trim();
    const visibility = document.getElementById('queue-visibility').value;
    const body = {name: name, visibility: visibility};
    if (prefix) body.prefix = prefix;
    if (dept)   body.department = dept;
    if (color)  body.color = color;

    const res = await fetch('/api/orgs/' + currentOrgID + '/queues', {
        method: 'POST',
        headers: csrfHeaders(),
        body: JSON.stringify(body)
    });

    if (res.ok) {
        hideCreateQueue();
        document.getElementById('queue-name').value = '';
        document.getElementById('queue-prefix').value = '';
        document.getElementById('queue-dept').value = '';
        document.getElementById('queue-color').value = '';
        document.getElementById('queue-visibility').value = 'normal';
        loadSidebar();
    } else {
        const data = await res.json();
        showToast('error', data.error || 'Failed to create queue');
    }
}

// ── Tickets ──────────────────────────────────────────────────────────────────
async function loadTickets(filter) {
    if (!currentOrgID) return;
    currentFilter = filter || 'all';
    currentOffset = 0;

    let url = '/api/orgs/' + currentOrgID + '/tickets?limit=50&offset=0';
    if (currentFilter !== 'all') {
        url += '&status=' + currentFilter;
    }

    const res = await fetch(url);
    if (!res.ok) return;
    const data = await res.json();
    hasMoreTickets = data.has_more || false;
    renderTicketList(data.tickets || [], false);
    document.getElementById('load-more-btn').style.display = hasMoreTickets ? 'block' : 'none';
}

async function loadMoreTickets() {
    if (!currentOrgID || !hasMoreTickets) return;
    currentOffset += 50;

    let url = '/api/orgs/' + currentOrgID + '/tickets?limit=50&offset=' + currentOffset;
    if (currentFilter !== 'all') {
        url += '&status=' + currentFilter;
    }

    const res = await fetch(url);
    if (!res.ok) return;
    const data = await res.json();
    hasMoreTickets = data.has_more || false;
    renderTicketList(data.tickets || [], true);
    document.getElementById('load-more-btn').style.display = hasMoreTickets ? 'block' : 'none';
}

function filterTickets(filter, btn) {
    document.querySelectorAll('.filter-tab').forEach(function(t) {
        t.classList.remove('active');
    });
    btn.classList.add('active');

    if (currentQueueID) {
        loadQueueTickets(currentQueueID);
    } else {
        loadTickets(filter);
    }
}

function statusDisplayName(status) {
    const labels = {
        open:             'Open',
        assigned:         'Assigned',
        in_progress:      'In Progress',
        pending_customer: 'Pending Customer',
        pending_vendor:   'Pending Vendor',
        resolved:         'Resolved',
        closed:           'Closed',
        reopened:         'Reopened'
    };
    return labels[status] || status.replace(/_/g, ' ');
}

function validTransitions(status) {
    const map = {
        open:             ['assigned', 'closed'],
        assigned:         ['open', 'in_progress', 'pending_customer', 'pending_vendor', 'resolved', 'closed'],
        in_progress:      ['assigned', 'pending_customer', 'pending_vendor', 'resolved', 'closed'],
        pending_customer: ['in_progress', 'resolved', 'closed'],
        pending_vendor:   ['in_progress', 'resolved', 'closed'],
        resolved:         ['reopened', 'closed'],
        closed:           [],
        reopened:         ['assigned', 'in_progress', 'resolved', 'closed']
    };
    return map[status] || [];
}

function statusActionLabel(currentStatus, nextStatus) {
    if (nextStatus === 'open')     return 'Unassign';
    if (nextStatus === 'assigned') return currentStatus === 'in_progress' ? 'Hand Off' : 'Assign';
    const labels = {
        in_progress:      'In Progress',
        pending_customer: 'Pending Customer',
        pending_vendor:   'Pending Vendor',
        resolved:         'Resolve',
        closed:           'Close',
        reopened:         'Reopen'
    };
    return labels[nextStatus] || statusDisplayName(nextStatus);
}

function populateStatusSelect(currentStatus) {
    const select = document.getElementById('action-status');
    if (!select) return;
    select.innerHTML = '';
    const cur = document.createElement('option');
    cur.value = currentStatus;
    cur.textContent = statusDisplayName(currentStatus);
    select.appendChild(cur);
    validTransitions(currentStatus).forEach(function(next) {
        const opt = document.createElement('option');
        opt.value = next;
        opt.textContent = statusDisplayName(next);
        select.appendChild(opt);
    });
    select.value = currentStatus;
}

function renderTicketList(tickets, append) {
    const list = document.getElementById('ticket-list');
    if (!append) list.innerHTML = '';

    if (!tickets || tickets.length === 0) {
        if (!append) list.innerHTML = '<div class="empty">No tickets found. <button onclick="showCreateTicket()" class="btn-link">Create one?</button></div>';
        return;
    }

    const priorityOrder = {P0: 0, P1: 1, P2: 2, P3: 3};
    tickets.sort(function(a, b) {
        const pd = (priorityOrder[a.priority] || 99) - (priorityOrder[b.priority] || 99);
        if (pd !== 0) return pd;
        return new Date(a.created_at) - new Date(b.created_at);
    });

    tickets.forEach(function(t) {
        const div = document.createElement('div');
        div.className = 'ticket-card' + (t.priority === 'P0' ? ' ticket-card-critical' : '');
        div.onclick = function() { openTicket(t.id); };

        const left = document.createElement('div');
        left.className = 'ticket-card-left';

        const priority = document.createElement('span');
        priority.className = 'ticket-priority priority-' + t.priority.toLowerCase();
        priority.textContent = t.priority;
        left.appendChild(priority);

        const ticketId = document.createElement('span');
        ticketId.className = 'ticket-display-id';
        ticketId.textContent = t.display_id || t.id.substring(0, 8);
        left.appendChild(ticketId);

        const subject = document.createElement('span');
        subject.className = 'ticket-subject';
        subject.textContent = t.subject;
        left.appendChild(subject);

        const right = document.createElement('div');
        right.className = 'ticket-card-right';

        const status = document.createElement('span');
        status.className = 'ticket-status status-' + t.status;
        status.textContent = t.status.replace(/_/g, ' ');
        right.appendChild(status);

        const age = document.createElement('span');
        age.className = 'ticket-age';
        age.textContent = timeAgo(t.created_at);
        right.appendChild(age);

        if (t.sla_due_at) {
            const due = new Date(t.sla_due_at);
            const now = new Date();
            const breached = due < now;
            const diffH = Math.round((due - now) / 36e5);
            const sla = document.createElement('span');
            sla.className = 'ticket-sla';
            if (breached) {
                sla.textContent = 'SLA breached';
                sla.style.color = '#dc2626';
            } else if (diffH < 4) {
                sla.textContent = 'SLA ' + diffH + 'h';
                sla.style.color = '#d97706';
            } else if (diffH < 24) {
                sla.textContent = 'SLA ' + diffH + 'h';
                sla.style.color = '#57534e';
            } else {
                sla.textContent = 'SLA ' + Math.round(diffH / 24) + 'd';
                sla.style.color = '#57534e';
            }
            right.appendChild(sla);
        }

        // Action menu button
        const actions = document.createElement('div');
        actions.className = 'ticket-actions';
        actions.onclick = function(e) {
            e.stopPropagation();
            closeAllMenus();
            const menu = actions.querySelector('.ticket-menu');
            menu.style.display = menu.style.display === 'block' ? 'none' : 'block';
        };
        actions.innerHTML = '<span class="ticket-actions-btn">⋯</span>';

        const menu = document.createElement('div');
        menu.className = 'ticket-menu';

        const role = (sidebarData && sidebarData.role) || 'viewer';

        const assignItem = document.createElement('div');
        assignItem.className = 'ticket-menu-item';
        assignItem.textContent = 'Assign to me';
        assignItem.onclick = function(e) { e.stopPropagation(); quickAssignToMe(t.id); };
        menu.appendChild(assignItem);

        validTransitions(t.status).forEach(function(nextStatus) {
            const item = document.createElement('div');
            item.className = 'ticket-menu-item';
            item.textContent = statusActionLabel(t.status, nextStatus);
            item.onclick = function(e) { e.stopPropagation(); quickStatus(t.id, nextStatus); };
            menu.appendChild(item);
        });

        if (role === 'owner' || role === 'admin') {
            const divider = document.createElement('div');
            divider.className = 'ticket-menu-divider';
            menu.appendChild(divider);
            const deleteItem = document.createElement('div');
            deleteItem.className = 'ticket-menu-item ticket-menu-danger';
            deleteItem.textContent = 'Delete';
            deleteItem.onclick = function(e) { e.stopPropagation(); quickDelete(t.id); };
            menu.appendChild(deleteItem);
        }

        actions.appendChild(menu);

        div.appendChild(left);
        div.appendChild(right);
        div.appendChild(actions);
        list.appendChild(div);
    });
}

function closeAllMenus() {
    document.querySelectorAll('.ticket-menu').forEach(function(m) { m.style.display = 'none'; });
}
document.addEventListener('click', closeAllMenus);

async function quickAssignToMe(ticketID) {
    closeAllMenus();
    const res = await fetch('/api/auth/me');
    if (!res.ok) return;
    const user = await res.json();
    const r = await fetch('/api/orgs/' + currentOrgID + '/tickets/' + ticketID + '/assign', {
        method: 'POST',
        headers: csrfHeaders(),
        body: JSON.stringify({assignee_user_id: user.id})
    });
    if (r.ok) { loadTickets(currentFilter); loadSidebar(); }
    else { const d = await r.json(); showToast('error', d.error || 'Action failed'); }
}

async function quickStatus(ticketID, status) {
    closeAllMenus();
    const res = await fetch('/api/orgs/' + currentOrgID + '/tickets/' + ticketID + '/status', {
        method: 'POST',
        headers: csrfHeaders(),
        body: JSON.stringify({status: status})
    });
    if (res.ok) {
        loadTickets(currentFilter);
        loadSidebar();
        return;
    }
    let message = 'Status change failed';
    try {
        const d = await res.json();
        if (d.error) {
            message = d.error.toLowerCase().includes('transition')
                ? 'That status change is not allowed from the current state.'
                : d.error;
        }
    } catch (_) {}
    showToast('error', message);
}

async function quickDelete(ticketID) {
    closeAllMenus();
    if (!confirm('Delete this ticket? This cannot be undone.')) return;
    // TODO: implement delete endpoint
    showToast('info', 'Delete not yet implemented');
}

function showCreateTicket() {
    document.getElementById('create-ticket-form').style.display = 'flex';
    populateQueueDropdown();
}

async function populateQueueDropdown() {
    const select = document.getElementById('ticket-queue');
    if (!select || !currentOrgID) return;

    select.innerHTML = '<option value="">No Queue</option>';

    try {
        const res = await fetch('/api/orgs/' + currentOrgID + '/queues/sidebar');
        if (!res.ok) return;
        const data = await res.json();
        (data.queues || []).forEach(function(q) {
            const opt = document.createElement('option');
            opt.value = q.id;
            opt.textContent = q.name;
            if (currentQueueID && currentQueueID === q.id) opt.selected = true;
            select.appendChild(opt);
        });
    } catch (err) {}
}
function hideCreateTicket()  { document.getElementById('create-ticket-form').style.display = 'none'; }

async function createTicket() {
    if (!currentOrgID) return;
    const customerEmail = document.getElementById('ticket-customer-email').value.trim();
    const subject  = document.getElementById('ticket-subject').value.trim();
    const body     = document.getElementById('ticket-body').value.trim();
    const priority = document.getElementById('ticket-priority').value;
    const queueID = document.getElementById('ticket-queue').value || null;
    if (!subject || !body) { showToast('warn', 'Subject and description are required'); return; }

    const res = await fetch('/api/orgs/' + currentOrgID + '/tickets', {
        method: 'POST',
        headers: csrfHeaders(),
        body: JSON.stringify({subject: subject, body: body, priority: priority, source_type: 'web', customer_email: customerEmail, queue_id: queueID})
    });

    if (res.ok) {
        hideCreateTicket();
        document.getElementById('ticket-subject').value = '';
        document.getElementById('ticket-body').value = '';
        document.getElementById('ticket-customer-email').value = '';
        loadSidebar();
        loadTickets(currentFilter); showView('queue');
    } else {
        const data = await res.json();
        showToast('error', data.error || 'Failed to create ticket');
    }
}

// ── Ticket Detail ─────────────────────────────────────────────────────────────
async function openTicket(ticketID) {
    if (!currentOrgID) return;
    currentTicketID = ticketID;

    const res = await fetch('/api/orgs/' + currentOrgID + '/tickets/' + ticketID);
    if (!res.ok) { showToast('error', 'Failed to load ticket'); return; }
    const ctx = await res.json();
    const t = ctx.ticket;

    const priorityEl = document.getElementById('detail-priority');
    priorityEl.className = 'ticket-priority priority-' + t.priority.toLowerCase();
    priorityEl.textContent = t.priority;

    const statusEl = document.getElementById('detail-status');
    statusEl.className = 'ticket-status status-' + t.status;
    statusEl.textContent = t.status.replace(/_/g, ' ');

    document.getElementById('detail-subject').textContent = (t.display_id || t.id.substring(0, 8)) + '  ' + t.subject;
    document.getElementById('detail-created').textContent = 'Opened ' + timeAgo(t.created_at);
    document.getElementById('detail-source').textContent = 'via ' + t.source_type;
    document.getElementById('detail-body').textContent = t.body;

    // SLA due date — show only if present
    const slaEl = document.getElementById('detail-sla');
    const slaDotEl = document.querySelector('.detail-sla-dot');
    if (t.sla_due_at) {
        const due = new Date(t.sla_due_at);
        const now = new Date();
        const breached = due < now;
        const diffMs = due - now;
        const diffH = Math.round(diffMs / 36e5);
        let slaText;
        if (breached) {
            slaText = 'SLA breached';
        } else if (diffH < 1) {
            slaText = 'SLA due < 1h';
        } else if (diffH < 24) {
            slaText = 'SLA due in ' + diffH + 'h';
        } else {
            const diffD = Math.round(diffH / 24);
            slaText = 'SLA due in ' + diffD + 'd';
        }
        slaEl.textContent = slaText;
        slaEl.style.color = breached ? '#dc2626' : (diffH < 4 ? '#d97706' : '#57534e');
        slaEl.style.display = '';
        slaDotEl.style.display = '';
    } else {
        slaEl.textContent = '';
        slaEl.style.display = 'none';
        slaDotEl.style.display = 'none';
    }

    populateStatusSelect(t.status);
    document.getElementById('action-priority').value = t.priority;

    renderComments(ctx.comments || []);
    renderEvents(ctx.events || []);


    showView('detail');
}

function renderComments(comments) {
    const thread = document.getElementById('comment-thread');
    thread.innerHTML = '';

    if (!comments || comments.length === 0) {
        thread.innerHTML = '<p class="empty-thread">No comments yet. Add the first response.</p>';
        return;
    }

    comments.forEach(function(c) {
        const div = document.createElement('div');
        div.className = 'comment' + (c.is_internal ? ' comment-internal' : '');

        const meta = document.createElement('div');
        meta.className = 'comment-meta';

        const who = document.createElement('span');
        who.className = 'comment-author';
        who.textContent = c.author_id ? 'Agent' : 'Customer';
        meta.appendChild(who);

        if (c.is_internal) {
            const badge = document.createElement('span');
            badge.className = 'comment-internal-badge';
            badge.textContent = 'Internal';
            meta.appendChild(badge);
        }

        const when = document.createElement('span');
        when.className = 'comment-time';
        when.textContent = timeAgo(c.created_at);
        meta.appendChild(when);

        const body = document.createElement('div');
        body.className = 'comment-body';
        body.textContent = c.body;

        div.appendChild(meta);
        div.appendChild(body);
        thread.appendChild(div);
    });
}

function renderEvents(events) {
    const trail = document.getElementById('event-trail');
    trail.innerHTML = '';

    if (!events || events.length === 0) {
        trail.innerHTML = '<p class="empty-thread">No activity yet.</p>';
        return;
    }

    const recent = events.slice().reverse().slice(0, 10);
    recent.forEach(function(e) {
        const div = document.createElement('div');
        div.className = 'event-row';

        const type = document.createElement('span');
        type.className = 'event-type';
        type.textContent = e.event_type.replace(/\./g, ' ');
        div.appendChild(type);

        const when = document.createElement('span');
        when.className = 'event-time';
        when.textContent = timeAgo(e.created_at);
        div.appendChild(when);

        trail.appendChild(div);
    });
}

async function changeStatus(newStatus) {
    if (!currentOrgID || !currentTicketID) return;

    const res = await fetch(
        '/api/orgs/' + currentOrgID + '/tickets/' + currentTicketID + '/status',
        {method: 'POST', headers: csrfHeaders(),
         body: JSON.stringify({status: newStatus})}
    );

    if (res.ok) {
        const statusEl = document.getElementById('detail-status');
        statusEl.className = 'ticket-status status-' + newStatus;
        statusEl.textContent = newStatus.replace(/_/g, ' ');
        populateStatusSelect(newStatus);
        loadSidebar();
    } else {
        const data = await res.json();
        showToast('error', data.error || 'Invalid status transition');
        openTicket(currentTicketID);
    }
}

async function changePriority(newPriority) {
    if (!currentOrgID || !currentTicketID) return;

    const res = await fetch(
        '/api/orgs/' + currentOrgID + '/tickets/' + currentTicketID + '/priority',
        {method: 'POST', headers: csrfHeaders(),
         body: JSON.stringify({priority: newPriority})}
    );

    if (!res.ok) {
        const data = await res.json();
        showToast('error', data.error || 'Failed to update priority');
        openTicket(currentTicketID);
    } else {
        loadSidebar();
    }
}

async function addComment() {
    if (!currentOrgID || !currentTicketID) return;
    const body       = document.getElementById('comment-body').value.trim();
    const isInternal = document.getElementById('comment-internal').checked;
    if (!body) return;

    const res = await fetch(
        '/api/orgs/' + currentOrgID + '/tickets/' + currentTicketID + '/comments',
        {method: 'POST', headers: csrfHeaders(),
         body: JSON.stringify({body: body, is_internal: isInternal})}
    );

    if (res.ok) {
        document.getElementById('comment-body').value = '';
        document.getElementById('comment-internal').checked = false;
        openTicket(currentTicketID);
    } else {
        const data = await res.json();
        showToast('error', data.error || 'Failed to add comment');
    }
}

// ── Logout ────────────────────────────────────────────────────────────────────
async function handleLogout() {
    await fetch('/api/auth/logout', {method: 'POST', headers: {'X-CSRF-Token': getCSRFToken()}});
    window.location.href = '/login';
}

// ── Helpers ───────────────────────────────────────────────────────────────────

function escapeHTML(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}

function icon(name, size) {
    const cls = size ? 'icon icon-' + size : 'icon';
    return '<svg class="' + cls + '"><use href="/static/icons/sprite.svg#icon-' + name + '"></use></svg>';
}

var CATEGORY_ICONS = {
    'Hardware': 'laptop',
    'Software': 'package',
    'Access': 'key',
    'Network': 'wifi',
    'Security': 'shield',
    'People': 'users',
    'Facilities': 'building',
    'Email': 'mail',
    'General': 'clipboard-list',
    'Bug': 'bug',
    'Maintenance': 'wrench',
    'Training': 'graduation-cap',
    'Infrastructure': 'hard-drive',
    'Printing': 'printer',
    'Telecom': 'phone'
};

function categoryIcon(category) {
    return CATEGORY_ICONS[category] || 'clipboard-list';
}

function showToast(type, message) {
    const container = document.getElementById('toast-container');
    const toast = document.createElement('div');
    toast.className = 'toast toast-' + type;
    toast.textContent = message;
    container.appendChild(toast);
    setTimeout(function() {
        toast.style.animation = 'toastOut 0.25s ease-in forwards';
        setTimeout(function() { toast.remove(); }, 250);
    }, 3500);
}

function timeAgo(dateStr) {
    const now  = new Date();
    const then = new Date(dateStr);
    const sec  = Math.floor((now - then) / 1000);
    if (sec < 60)    return sec + 's ago';
    if (sec < 3600)  return Math.floor(sec / 60) + 'm ago';
    if (sec < 86400) return Math.floor(sec / 3600) + 'h ago';
    return Math.floor(sec / 86400) + 'd ago';
}


// ── Org Settings ─────────────────────────────────────────────────────────────
async function loadOrgSettings() {
    if (!currentOrgID) return;
    const res = await fetch('/api/orgs/' + currentOrgID);
    if (!res.ok) return;
    const data = await res.json();
    const org = data.org || data;
    document.getElementById('settings-org-name').value = org.name || '';
    document.getElementById('settings-org-domain').value = org.domain || '';
    document.getElementById('settings-org-inbound').value = org.inbound_email || '';
    loadMembers();
}

async function saveOrgSettings() {
    if (!currentOrgID) return;
    const name = document.getElementById('settings-org-name').value.trim();
    const domain = document.getElementById('settings-org-domain').value.trim();
    const inbound = document.getElementById('settings-org-inbound').value.trim();
    const statusEl = document.getElementById('settings-org-status');

    const body = {};
    if (name) body.name = name;
    if (domain) body.domain = domain;
    if (inbound) body.inbound_email = inbound;

    const res = await fetch('/api/orgs/' + currentOrgID + '/settings', {
        method: 'POST',
        headers: csrfHeaders(),
        body: JSON.stringify(body)
    });

    if (res.ok) {
        statusEl.textContent = '✓ Saved';
        statusEl.style.color = 'var(--status-resolved)';
        setTimeout(function() { statusEl.textContent = ''; }, 2000);
        // Update nav org name
        document.getElementById('nav-org-name').textContent = name;
        currentOrgName = name;
    } else {
        const d = await res.json();
        statusEl.textContent = d.error || 'Failed to save';
        statusEl.style.color = 'var(--priority-p0)';
    }
}

async function loadMembers() {
    if (!currentOrgID) return;
    const res = await fetch('/api/orgs/' + currentOrgID);
    if (!res.ok) return;
    const data = await res.json();
    const members = data.members || [];
    const list = document.getElementById('settings-members-list');
    list.innerHTML = '';

    if (members.length === 0) {
        list.innerHTML = '<p class="empty">No members found.</p>';
        return;
    }

    members.forEach(function(m) {
        const row = document.createElement('div');
        row.className = 'settings-member-row';
        row.innerHTML =
            '<span class="settings-member-name">' + escapeHTML(m.handle || m.email) + '</span>' +
            '<span class="settings-member-email">' + escapeHTML(m.email) + '</span>' +
            '<span class="org-role">' + escapeHTML(m.role) + '</span>';
        list.appendChild(row);
    });
}

async function inviteMember() {
    if (!currentOrgID) return;
    const email = document.getElementById('settings-invite-email').value.trim();
    const role = document.getElementById('settings-invite-role').value;
    if (!email) { showToast('warn', 'Email is required'); return; }

    const res = await fetch('/api/orgs/' + currentOrgID + '/members', {
        method: 'POST',
        headers: csrfHeaders(),
        body: JSON.stringify({email: email, role: role})
    });

    if (res.ok) {
        document.getElementById('settings-invite-email').value = '';
        loadMembers();
    } else {
        const d = await res.json();
        showToast('error', d.error || 'Failed to invite');
    }
}


// ── Queue Settings ───────────────────────────────────────────────────────────
async function loadQueueSettings() {
    if (!currentOrgID || !currentSettingsQueueID) return;
    const res = await fetch('/api/orgs/' + currentOrgID + '/queues/' + currentSettingsQueueID);
    if (!res.ok) return;
    const data = await res.json();
    const q = data.queue;
    document.getElementById('queue-settings-title').textContent = 'Queue: ' + q.name;
    document.getElementById('settings-queue-name').value = q.name || '';
    document.getElementById('settings-queue-prefix').value = q.prefix || '';
    document.getElementById('settings-queue-dept').value = q.department || '';
    document.getElementById('settings-queue-color').value = q.color || '';
    renderQueueMembers(data.members || []);
    loadOrgMembersForSelect();
    loadCatalogItems();
}

async function saveQueueSettings() {
    if (!currentOrgID || !currentSettingsQueueID) return;
    const prefix = document.getElementById('settings-queue-prefix').value.trim();
    const name = document.getElementById('settings-queue-name').value.trim();
    const dept = document.getElementById('settings-queue-dept').value.trim();
    const color = document.getElementById('settings-queue-color').value.trim();
    const statusEl = document.getElementById('settings-queue-status');

    const body = {};
    if (prefix) body.prefix = prefix;
    if (name) body.name = name;
    if (dept) body.department = dept;
    if (color) body.color = color;

    const res = await fetch('/api/orgs/' + currentOrgID + '/queues/' + currentSettingsQueueID + '/settings', {
        method: 'POST',
        headers: csrfHeaders(),
        body: JSON.stringify(body)
    });

    if (res.ok) {
        statusEl.textContent = '✓ Saved';
        statusEl.style.color = 'var(--status-resolved)';
        setTimeout(function() { statusEl.textContent = ''; }, 2000);
        loadSidebar();
    } else {
        const d = await res.json();
        statusEl.textContent = d.error || 'Failed to save';
        statusEl.style.color = 'var(--priority-p0)';
    }
}

function renderQueueMembers(members) {
    const list = document.getElementById('queue-members-list');
    list.innerHTML = '';
    if (members.length === 0) {
        list.innerHTML = '<p class="empty">No team members yet. Add agents below.</p>';
        return;
    }
    members.forEach(function(m) {
        const row = document.createElement('div');
        row.className = 'settings-member-row';
        row.innerHTML =
            '<span class="settings-member-name">' + m.user_id.substring(0, 8) + '...</span>' +
            '<span class="org-role">' + escapeHTML(m.role) + '</span>' +
            '<button class="btn btn-ghost btn-sm" onclick="removeQueueMember(\'' + m.user_id + '\')">' + icon('x', 'sm') + '</button>';
        list.appendChild(row);
    });
}

async function loadOrgMembersForSelect() {
    if (!currentOrgID) return;
    const res = await fetch('/api/orgs/' + currentOrgID);
    if (!res.ok) return;
    const data = await res.json();
    const members = data.members || [];
    const select = document.getElementById('queue-member-select');
    select.innerHTML = '<option value="">Select agent...</option>';
    members.forEach(function(m) {
        const opt = document.createElement('option');
        opt.value = m.user_id;
        opt.textContent = (m.handle || m.email) + ' (' + m.role + ')';
        select.appendChild(opt);
    });
}

async function addQueueMember() {
    if (!currentOrgID || !currentSettingsQueueID) return;
    const userID = document.getElementById('queue-member-select').value;
    const role = document.getElementById('queue-member-role').value;
    if (!userID) { showToast('warn', 'Select an agent'); return; }

    const res = await fetch('/api/orgs/' + currentOrgID + '/queues/' + currentSettingsQueueID + '/members', {
        method: 'POST',
        headers: csrfHeaders(),
        body: JSON.stringify({user_id: userID, role: role})
    });

    if (res.ok) {
        loadQueueSettings();
    } else {
        const d = await res.json();
        showToast('error', d.error || 'Failed to add member');
    }
}

async function removeQueueMember(userID) {
    if (!currentOrgID || !currentSettingsQueueID) return;
    if (!confirm('Remove this member from the queue?')) return;

    const res = await fetch('/api/orgs/' + currentOrgID + '/queues/' + currentSettingsQueueID + '/members/' + userID, {
        method: 'DELETE',
        headers: {'X-CSRF-Token': getCSRFToken()}
    });

    if (res.ok) {
        loadQueueSettings();
    } else {
        const d = await res.json();
        showToast('error', d.error || 'Failed to remove member');
    }
}


// ── Account Settings ─────────────────────────────────────────────────────────
async function loadAccountSettings() {
    const res = await fetch('/api/auth/me');
    if (!res.ok) return;
    const user = await res.json();
    document.getElementById('settings-handle').value = user.handle || '';
    document.getElementById('settings-email').value = user.email || '';
    document.getElementById('settings-mfa-status').textContent = user.mfa_enabled ? 'Enabled ✓' : 'Not enabled';
    loadPasskeys();
}

async function loadPasskeys() {
    const list = document.getElementById('passkey-list');
    list.innerHTML = '';

    try {
        const res = await fetch('/api/auth/passkeys');
        if (!res.ok) {
            list.innerHTML = '<p class="settings-hint">Failed to load passkeys.</p>';
            return;
        }
        const data = await res.json();
        const passkeys = data.passkeys || [];

        if (passkeys.length === 0) {
            list.innerHTML = '<p class="settings-hint">No passkeys registered yet. Add one below.</p>';
            return;
        }

        passkeys.forEach(function(pk) {
            const row = document.createElement('div');
            row.className = 'settings-member-row';

            const name = document.createElement('span');
            name.className = 'settings-member-name';
            name.textContent = pk.name || 'Passkey';

            const meta = document.createElement('span');
            meta.className = 'settings-hint';
            meta.style.marginLeft = '0.5rem';
            const created = timeAgo(pk.created_at);
            const lastUsed = pk.last_used_at ? timeAgo(pk.last_used_at) : 'never';
            meta.textContent = 'Added ' + created + ' · Last used: ' + lastUsed + ' · Sign count: ' + pk.sign_count;

            const deleteBtn = document.createElement('button');
            deleteBtn.className = 'btn btn-ghost btn-sm';
            deleteBtn.innerHTML = icon('x', 'sm');
            deleteBtn.onclick = function() { deletePasskey(pk.id, pk.name || 'Passkey'); };

            row.appendChild(name);
            row.appendChild(meta);
            row.appendChild(deleteBtn);
            list.appendChild(row);
        });
    } catch (err) {
        list.innerHTML = '<p class="settings-hint">Failed to load passkeys.</p>';
    }
}

async function deletePasskey(passkeyID, name) {
    if (!confirm('Remove passkey "' + name + '"? You will no longer be able to sign in with it.')) return;

    try {
        const res = await fetch('/api/auth/passkeys/delete?id=' + passkeyID, {method: 'DELETE', headers: {'X-CSRF-Token': getCSRFToken()}});
        if (res.ok) {
            showToast('success', 'Passkey removed');
            loadPasskeys();
        } else {
            const d = await res.json();
            showToast('error', d.error || 'Failed to remove passkey');
        }
    } catch (err) {
        showToast('error', 'Failed to remove passkey');
    }
}

async function registerPasskey() {
    const statusEl = document.getElementById('passkey-status');
    statusEl.textContent = 'Starting...';
    statusEl.style.color = 'var(--text-secondary)';

    try {
        // Step 1: Begin registration
        const beginRes = await fetch('/api/auth/passkey/register/begin', {method: 'POST', headers: {'X-CSRF-Token': getCSRFToken()}});
        if (!beginRes.ok) {
            const d = await beginRes.json();
            statusEl.textContent = d.error || 'Failed to start';
            statusEl.style.color = 'var(--priority-p0)';
            return;
        }

        const options = await beginRes.json();

        // Convert base64url fields to ArrayBuffers
        options.publicKey.challenge = base64urlToBuffer(options.publicKey.challenge);
        options.publicKey.user.id = base64urlToBuffer(options.publicKey.user.id);
        if (options.publicKey.excludeCredentials) {
            options.publicKey.excludeCredentials = options.publicKey.excludeCredentials.map(function(c) {
                c.id = base64urlToBuffer(c.id);
                return c;
            });
        }

        // Step 2: Browser creates credential
        statusEl.textContent = 'Waiting for device...';
        const credential = await navigator.credentials.create({publicKey: options.publicKey});

        // Step 3: Send attestation to server
        const attestation = {
            id: credential.id,
            rawId: bufferToBase64url(credential.rawId),
            type: credential.type,
            response: {
                attestationObject: bufferToBase64url(credential.response.attestationObject),
                clientDataJSON: bufferToBase64url(credential.response.clientDataJSON)
            }
        };

        const finishRes = await fetch('/api/auth/passkey/register/finish', {
            method: 'POST',
            headers: csrfHeaders(),
            body: JSON.stringify(attestation)
        });

        if (finishRes.ok) {
            statusEl.textContent = '';
            showToast('success', 'Passkey registered successfully');
            loadPasskeys();
        } else {
            const d = await finishRes.json();
            statusEl.textContent = d.error || 'Registration failed';
            statusEl.style.color = 'var(--priority-p0)';
        }
    } catch (err) {
        statusEl.textContent = 'Cancelled or not supported';
        statusEl.style.color = 'var(--priority-p0)';
    }
}

function base64urlToBuffer(base64url) {
    const base64 = base64url.replace(/-/g, '+').replace(/_/g, '/');
    const pad = base64.length % 4;
    const padded = pad ? base64 + '='.repeat(4 - pad) : base64;
    const binary = atob(padded);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
    return bytes.buffer;
}

function bufferToBase64url(buffer) {
    const bytes = new Uint8Array(buffer);
    let binary = '';
    for (let i = 0; i < bytes.length; i++) binary += String.fromCharCode(bytes[i]);
    return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}


// ── Service Catalog ──────────────────────────────────────────────────────────
function showAddCatalogItem() {
    document.getElementById('catalog-add-form').style.display = 'block';
}
function hideCatalogForm() {
    document.getElementById('catalog-add-form').style.display = 'none';
}

async function loadCatalogItems() {
    if (!currentOrgID || !currentSettingsQueueID) return;
    const res = await fetch('/api/orgs/' + currentOrgID + '/catalog?queue_id=' + currentSettingsQueueID);
    if (!res.ok) return;
    const data = await res.json();
    renderCatalogItems(data.items || []);
}

function renderCatalogItems(items) {
    const list = document.getElementById('catalog-items-list');
    list.innerHTML = '';
    if (items.length === 0) {
        list.innerHTML = '<p class="empty">No services yet. Add your first service above.</p>';
        return;
    }

    items.forEach(function(item) {
        const row = document.createElement('div');
        row.className = 'settings-member-row';
        const typeMap = {service_request:'SR', incident:'INC', change:'CHG', problem:'PRB', task:'TSK'};
        const typeCode = typeMap[item.ticket_type] || item.ticket_type;
        row.innerHTML =
            '<span style="margin-right:0.5rem;display:inline-flex">' + icon(categoryIcon(item.category || 'General'), 'md') + '</span>' +
            '<span class="settings-member-name" style="flex:1">' + escapeHTML(item.name) + '</span>' +
            '<span class="settings-hint" style="margin-right:0.5rem">' + escapeHTML(item.category || '') + '</span>' +
            '<span class="org-role">' + escapeHTML(typeCode) + '</span>' +
            '<span class="org-role" style="margin-left:0.25rem">' + escapeHTML(item.default_priority) + '</span>' +
            (item.sla_hours ? '<span class="settings-hint" style="margin-left:0.25rem">' + item.sla_hours + 'h SLA</span>' : '') +
            '<button class="btn btn-ghost btn-sm" style="margin-left:0.5rem" onclick="deleteCatalogItem(\'' + item.id + '\')">' + icon('x', 'sm') + '</button>';
        list.appendChild(row);
    });
}

async function createCatalogItem() {
    if (!currentOrgID || !currentSettingsQueueID) return;
    const name = document.getElementById('catalog-item-name').value.trim();
    const desc = document.getElementById('catalog-item-desc').value.trim();
    const category = document.getElementById('catalog-item-category').value.trim();
    const ticketType = document.getElementById('catalog-item-type').value;
    const priority = document.getElementById('catalog-item-priority').value;
    const slaStr = document.getElementById('catalog-item-sla').value.trim();

    if (!name) { showToast('warn', 'Service name is required'); return; }

    const body = {
        queue_id: currentSettingsQueueID,
        name: name,
        ticket_type: ticketType,
        default_priority: priority
    };
    if (desc) body.description = desc;
    if (category) body.category = category;
    if (slaStr) body.sla_hours = parseInt(slaStr);

    const res = await fetch('/api/orgs/' + currentOrgID + '/catalog', {
        method: 'POST',
        headers: csrfHeaders(),
        body: JSON.stringify(body)
    });

    if (res.ok) {
        hideCatalogForm();
        document.getElementById('catalog-item-name').value = '';
        document.getElementById('catalog-item-desc').value = '';
        document.getElementById('catalog-item-category').value = '';
        document.getElementById('catalog-item-sla').value = '';
        loadCatalogItems();
        showToast('success', 'Service created');
    } else {
        const d = await res.json();
        showToast('error', d.error || 'Failed to create service');
    }
}

async function deleteCatalogItem(itemID) {
    if (!currentOrgID) return;
    if (!confirm('Remove this service from the catalog?')) return;

    const res = await fetch('/api/orgs/' + currentOrgID + '/catalog/' + itemID, {
        method: 'DELETE',
        headers: {'X-CSRF-Token': getCSRFToken()}
    });

    if (res.ok) {
        loadCatalogItems();
        showToast('success', 'Service removed');
    } else {
        const d = await res.json();
        showToast('error', d.error || 'Failed to remove service');
    }
}

init();
// ── 404NOT403 · Auth UI ──────────────────────────────────────────────────────
// Uses HttpOnly session cookie. Frontend never touches the token.
// Session state is discovered via /api/auth/me.

// ── Modal controls ────────────────────────────────────────────────────────────
function showAuthModal(mode) {
    var modal = document.getElementById('auth-modal');
    var title = document.getElementById('modal-title');
    var loginForm = document.getElementById('login-form');
    var registerForm = document.getElementById('register-form');
    var mfaGroup = document.getElementById('mfa-group');
    var loginError = document.getElementById('login-error');
    var registerError = document.getElementById('register-error');

    loginError.textContent = '';
    registerError.textContent = '';
    if (mfaGroup) mfaGroup.style.display = 'none';

    if (mode === 'login') {
        title.textContent = 'SIGN IN';
        loginForm.style.display = 'block';
        registerForm.style.display = 'none';
    } else {
        title.textContent = 'CREATE ACCOUNT';
        loginForm.style.display = 'none';
        registerForm.style.display = 'block';
    }

    modal.style.display = 'flex';
}

function closeAuthModal() {
    var modal = document.getElementById('auth-modal');
    modal.style.display = 'none';
}

// ── Register ─────────────────────────────────────────────────────────────────
async function register() {
    var email = document.getElementById('register-email').value.trim();
    var handle = document.getElementById('register-handle').value.trim();
    var password = document.getElementById('register-password').value;
    var errEl = document.getElementById('register-error');

    errEl.textContent = '';

    try {
        var response = await fetch('/api/auth/register', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'same-origin',
            body: JSON.stringify({
                email: email,
                handle: handle,
                password: password
            }),
        });

        var data = await response.json();

        if (!response.ok) {
            errEl.textContent = data.error || 'Registration failed';
            return;
        }

        closeAuthModal();
        await refreshSessionUI();

    } catch (err) {
        errEl.textContent = 'Registration failed';
    }
}

// ── Login ────────────────────────────────────────────────────────────────────
async function login() {
    var identifier = document.getElementById('login-email').value.trim();
    var password = document.getElementById('login-password').value;
    var mfaCode = document.getElementById('login-mfa').value.trim();
    var errEl = document.getElementById('login-error');
    var mfaGroup = document.getElementById('mfa-group');

    errEl.textContent = '';

    try {
        var response = await fetch('/api/auth/login', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'same-origin',
            body: JSON.stringify({
                identifier: identifier,
                password: password,
                mfa_code: mfaCode
            }),
        });

        var data = await response.json();

        if (data.mfa_required) {
            mfaGroup.style.display = 'block';
            errEl.textContent = 'MFA code required';
            return;
        }

        if (!response.ok) {
            errEl.textContent = data.error || 'Login failed';
            return;
        }

        closeAuthModal();
        await refreshSessionUI();

    } catch (err) {
        errEl.textContent = 'Login failed';
    }
}

// ── Logout ───────────────────────────────────────────────────────────────────
async function logout() {
    try {
        await fetch('/api/auth/logout', {
            method: 'POST',
            credentials: 'same-origin'
        });
    } finally {
        await refreshSessionUI();
    }
}

// ── Session check ────────────────────────────────────────────────────────────
async function refreshSessionUI() {
    var loggedOut = document.getElementById('auth-logged-out');
    var loggedIn = document.getElementById('auth-logged-in');
    var authHandle = document.getElementById('auth-handle');
    var authRole = document.getElementById('auth-role');

    var gate = document.getElementById('monitor-auth-gate');
    var controls = document.getElementById('monitor-controls');

    try {
        var response = await fetch('/api/auth/me', {
            method: 'GET',
            credentials: 'same-origin'
        });

        if (!response.ok) throw new Error('not authenticated');

        var me = await response.json();

        // Auth bar
        loggedOut.style.display = 'none';
        loggedIn.style.display = 'flex';
        authHandle.textContent = me.handle;
	var roleMap = {
            observer: 'OBSERVER · FREE TIER',
            analyst:  'ANALYST · PRO',
            admin:    'ADMIN'
        };
        authRole.textContent = roleMap[me.role] || me.role.toUpperCase();

        // Show upgrade button only for free tier
        var upgradeBtn = document.getElementById('upgrade-btn');
        if (upgradeBtn) {
            upgradeBtn.style.display = (me.role === 'observer') ? 'inline-flex' : 'none';
        }
        // Monitor section
        gate.style.display = 'none';
        controls.style.display = 'block';

        // Load private data
        if (typeof loadMonitors === 'function') {
            loadMonitors();
        }

    } catch (err) {
        loggedOut.style.display = 'flex';
        loggedIn.style.display = 'none';

        gate.style.display = 'block';
        controls.style.display = 'none';
    }
}

// ── Handle availability check ────────────────────────────────────────────────
var handleTimer = null;

function checkHandleAvailability() {
    var input = document.getElementById('register-handle');
    var status = document.getElementById('handle-status');
    var handle = input.value.trim().toLowerCase();

    if (handleTimer) clearTimeout(handleTimer);

    if (handle.length < 3) {
        status.textContent = '';
        status.className = 'handle-status';
        return;
    }

    if (handle.length > 32) {
        status.textContent = 'TOO LONG';
        status.className = 'handle-status handle-taken';
        return;
    }

    status.textContent = 'CHECKING...';
    status.className = 'handle-status handle-checking';

    handleTimer = setTimeout(async function() {
        try {
            var response = await fetch('/api/auth/check-handle?handle=' + encodeURIComponent(handle));
            var data = await response.json();

            if (data.available) {
                status.textContent = 'AVAILABLE';
                status.className = 'handle-status handle-available';
            } else {
                status.textContent = data.reason ? data.reason.toUpperCase() : 'TAKEN';
                status.className = 'handle-status handle-taken';
            }
        } catch (err) {
            status.textContent = '';
            status.className = 'handle-status';
        }
    }, 500);
}
// ── Forgot Password ──────────────────────────────────────────────────────────
function showForgotPassword() {
    var title = document.getElementById('modal-title');
    var body = document.getElementById('modal-body');

    title.textContent = 'RESET PASSWORD';

    body.innerHTML = '';

    var group = document.createElement('div');
    group.className = 'form-group';

    var label = document.createElement('label');
    label.className = 'form-label';
    label.textContent = 'EMAIL';

    var input = document.createElement('input');
    input.type = 'email';
    input.id = 'forgot-email';
    input.className = 'form-input';
    input.placeholder = 'you@example.com';
    input.addEventListener('keydown', function(e) {
    if (e.key === 'Enter') sendResetLink();
});

    group.appendChild(label);
    group.appendChild(input);

    var errEl = document.createElement('div');
    errEl.className = 'form-error';
    errEl.id = 'forgot-error';

    var btn = document.createElement('button');
    btn.className = 'form-submit';
    btn.textContent = 'SEND RESET LINK';
    btn.onclick = sendResetLink;

    var back = document.createElement('p');
    back.className = 'form-switch';
    var backLink = document.createElement('a');
    backLink.href = '#';
    backLink.textContent = 'Back to sign in';
    backLink.onclick = function(e) {
        e.preventDefault();
        closeAuthModal();
        showAuthModal('login');
    };
    back.appendChild(backLink);

    body.appendChild(group);
    body.appendChild(errEl);
    body.appendChild(btn);
    body.appendChild(back);
}

async function sendResetLink() {
    var email = document.getElementById('forgot-email').value.trim();
    var errEl = document.getElementById('forgot-error');

    errEl.textContent = '';

    if (!email) {
        errEl.textContent = 'Enter your email address';
        return;
    }

    try {
        var response = await fetch('/api/auth/forgot', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ email: email }),
        });

        var data = await response.json();

        var body = document.getElementById('modal-body');
        body.innerHTML = '';

        var msg = document.createElement('div');
        msg.className = 'result-placeholder';
        var span = document.createElement('span');
        span.textContent = 'If an account exists, a reset link has been sent.';
        msg.appendChild(span);
        body.appendChild(msg);

        var back = document.createElement('p');
        back.className = 'form-switch';
        back.style.marginTop = '1rem';
        var backLink = document.createElement('a');
        backLink.href = '#';
        backLink.textContent = 'Back to sign in';
        backLink.onclick = function(e) {
            e.preventDefault();
            closeAuthModal();
            showAuthModal('login');
        };
        back.appendChild(backLink);
        body.appendChild(back);

    } catch (err) {
        errEl.textContent = 'Request failed';
    }
}
// ── Upgrade to Pro ───────────────────────────────────────────────────────────
async function upgradeToPro() {
    try {
        var response = await fetch('/api/billing/checkout', {
            method: 'POST',
            credentials: 'same-origin'
        });

        var data = await response.json();

        if (data.url) {
            window.location.href = data.url;
        } else {
            alert('Failed to start checkout');
        }
    } catch (err) {
        alert('Failed to connect to billing');
    }
}
// ── Modal dismiss on outside click ───────────────────────────────────────────
document.addEventListener('DOMContentLoaded', function() {
    var modal = document.getElementById('auth-modal');
    if (modal) {
        modal.addEventListener('click', function(e) {
            if (e.target === modal) closeAuthModal();
        });
    }

    refreshSessionUI();
	    // Enter key support for auth forms
    var loginPassword = document.getElementById('login-password');
    if (loginPassword) {
        loginPassword.addEventListener('keydown', function(e) {
            if (e.key === 'Enter') login();
        });
    }

    var loginEmail = document.getElementById('login-email');
    if (loginEmail) {
        loginEmail.addEventListener('keydown', function(e) {
            if (e.key === 'Enter') login();
        });
    }

    var registerPassword = document.getElementById('register-password');
    if (registerPassword) {
        registerPassword.addEventListener('keydown', function(e) {
            if (e.key === 'Enter') register();
        });
    }

 // Handle availability check
    var handleInput = document.getElementById('register-handle');
    if (handleInput) {
        handleInput.addEventListener('input', checkHandleAvailability);
    }
});

// ── Password toggle ──────────────────────────────────────────────────────────
function togglePassword(inputId, btn) {
    var input = document.getElementById(inputId);
    var icon = btn.querySelector('.eye-icon');
    if (input.type === 'password') {
        input.type = 'text';
        icon.textContent = '◎';
    } else {
        input.type = 'password';
        icon.textContent = '◉';
    }
}

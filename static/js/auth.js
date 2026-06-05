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
        authRole.textContent = ({observer:"FREE TIER",analyst:"PRO",admin:"ADMIN"})[me.role] || me.role.toUpperCase();

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

// ── Modal dismiss on outside click ───────────────────────────────────────────
document.addEventListener('DOMContentLoaded', function() {
    var modal = document.getElementById('auth-modal');
    if (modal) {
        modal.addEventListener('click', function(e) {
            if (e.target === modal) closeAuthModal();
        });
    }

    refreshSessionUI();
});

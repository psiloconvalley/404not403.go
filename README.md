# 404NOT403

> What did the server actually say?

**404NOT403** is a forensic HTTP instrument built to capture what a server actually returned — not what a browser inferred, cached, or hid.

It performs server-side HTTP inspection and monitoring for public URLs, with support for:
- raw status code and verdict analysis
- response header inspection
- TLS certificate analysis
- CDN detection
- WAF detection
- SHA256 body hash fingerprinting
- background URL monitoring with change detection
- authentication, billing, and user-scoped history

Live product: **https://404not403.com**

---

## Why It Exists

Most tools show you what the browser decided to render.

404NOT403 shows you what the server actually said.

That distinction matters when you are debugging:
- 403 vs WAF block
- 404 vs stale cache
- TLS expiry
- header misconfiguration
- content drift over time
- CDN and edge behavior
- third-party endpoint failures

---

## Core Features

### Header Inspector
Inspect any public URL with a server-side forensic scan.

Captures:
- HTTP status code
- status verdict
- response headers
- TLS issuer and expiry
- CDN provider
- WAF presence
- body fingerprint hash
- body size
- response duration
- scan region

### Ghost Link Monitor
Track URLs over time and detect when they change.

Supports:
- scheduled checks
- status transition detection
- content drift detection via body hash comparison
- user-scoped monitors
- recorded forensic evidence of change events

### Global Activity Feed
See recent forensic events across the platform.

Authenticated users also get:
- private scan history
- private monitor state
- detected change history

### Status Simulator
Trigger real 404 and 403 responses from the application to help users understand what each response means.

### Authentication + Billing
Includes:
- registration and login
- password reset via email
- RBAC
- Stripe checkout for paid tier upgrades

---

## Who It Is For

### Security Engineers
Validate:
- WAF behavior
- header security posture
- TLS certificate state
- origin vs edge response differences

### DevOps / SRE
Monitor:
- production endpoints
- third-party dependencies
- content drift
- silent failures over time

### Penetration Testers
Distinguish:
- true server-side 403 responses
- CDN interference
- edge filtering
- application-layer denial behavior

### Product Builders
Watch:
- competitor pages
- landing pages
- docs endpoints
- integrations your product depends on

### Investigators / Compliance Teams
Capture evidence of:
- what a URL returned
- when it changed
- whether content drift occurred

---

## Security Architecture

- **Passwords**: Argon2id
- **Sessions**: RS256 JWT
- **Cookies**: HttpOnly, Secure, SameSite=Strict
- **MFA**: TOTP support with AES-256-GCM encrypted secrets
- **API Keys**: generated once, stored hashed with SHA256
- **RBAC**: observer -> analyst -> admin
- **SSRF Protection**: private IP blocking, metadata endpoint blocking, DNS rebinding protection
- **XSS Prevention**: external data rendered with safe DOM APIs
- **Rate Limiting**: per-IP token bucket
- **CORS**: production origin only

---

## Stack

- **Backend**: Go
- **Database**: PostgreSQL
- **Frontend**: HTML, CSS, Vanilla JavaScript
- **Hosting**: Railway
- **Email**: Resend
- **Payments**: Stripe

Design system:
- dark forensic interface
- noir base
- teal accent
- red / amber HTTP verdict emphasis

---

## Architecture

### Request Flow

1. Client sends request
2. Railway routes traffic to Go service
3. Middleware handles:
   - logging
   - rate limiting
   - auth
   - RBAC
4. Route handler executes
5. Scanner engine performs forensic analysis
6. Results are persisted to PostgreSQL where applicable
7. Monitor worker performs scheduled checks in the background

### Scanner Engine
The scanner analyzes:
- status code
- headers
- TLS certificate metadata
- CDN signals
- WAF signals
- body fingerprint hash

### Monitor Worker
The monitor worker:
- wakes on interval
- checks due monitors
- compares previous state to current state
- records change evidence
- exits cleanly on shutdown via context cancellation

---

## Product Tiers

### Observer — Free
- 3 monitors max
- 24h interval only
- unlimited public scans
- global feed access

### Analyst — Pro — $19/month
- 50 monitors max
- 1h, 6h, or 24h intervals
- private scan history
- full monitor access

### Admin
- 500 monitors max
- all intervals
- reserved for future internal/admin controls

---

## Testing

Current automated coverage includes:
- password hashing tests
- API key generation tests
- scan engine tests
- SSRF protection tests

Total:
- **38 tests**
- **0 failures**

Run tests with:

```bash
go test ./...
undefined
go build ./...
Running Locally
Prerequisites
Go
PostgreSQL
required environment variables
Required Environment Variables
Bash

DATABASE_URL=
JWT_PRIVATE_KEY=
JWT_PUBLIC_KEY=
JWT_ISSUER=
MFA_ENCRYPTION_KEY=
RESEND_API_KEY=
STRIPE_SECRET_KEY=
STRIPE_PRICE_ID=
STRIPE_WEBHOOK_SECRET=
PORT=
Start
Bash

go build ./...
go test ./...
go run main.go
Routes
Public
GET /
GET /about
GET /status
GET /health
GET /simulate/404
GET /simulate/403
GET /api/stats
POST /api/scan
GET /api/feed
GET /reset
Auth
POST /api/auth/register
POST /api/auth/login
POST /api/auth/logout
GET /api/auth/me
GET /api/auth/check-handle
POST /api/auth/forgot
POST /api/auth/reset
Protected
GET /api/scans
POST /api/monitor
DELETE /api/monitor?id=
GET /api/monitors
GET /api/changes
POST /api/billing/checkout
Webhooks
POST /api/webhooks/stripe
Roadmap
Completed:

Layer 1: scan engine + database
Layer 2: header inspector + simulator + scan history
Layer 3: ghost link monitor
Layer 4: identity + auth + password reset
Layer 5: Stripe monetization
Next:

handler tests
store tests
subscription cancellation webhook
MFA UI
API key UI
multi-region scanning
API-first expansion
Built By
David Vasquez

Systems-focused builder with startup IT and infrastructure experience across identity, automation, endpoint management, and secure systems.

404NOT403 was built from first commit to production as a live forensic web product.

Links:

Live product: https://404not403.com
GitHub: https://github.com/psiloconvalley
Philosophy
Every layer answers one question at a different scale:

What did the server actually say?



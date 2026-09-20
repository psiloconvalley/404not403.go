# Routing Topology & Work Item Architecture

**Status:** Active
**Version:** 1.0
**Owners:** Founding Team
**Related Commits:** `cde3b92` (inbound email intake)

---

## 1. Strategic Vision & Core Thesis

> **The customer speaks the language of their everyday business requests. The operator speaks the language of professional service execution. The database serves as the translator between these two worlds, maintaining perfect consistency while altering the presentation layer to suit the audience.**

### Strategic Goals:
1. **Zero Setup Time:** Mirror company org charts directly into departments & queues.
2. **Invisible-to-End-User ITSM:** Employees submit via email and get email replies. Never log in.
3. **Deterministic Rules First, AI Second:** Keyword rules route instantly for free. AI handles nuance.
4. **Parent-Child Collaboration:** Multi-department workflows (e.g., Onboarding) split into sub-tasks.


---

## 2. Ubiquitous Language Dictionary

### Work Item Vocabulary
- **Ticket:** (Internal Only) Database-level representation. Used by agents and code. NEVER shown to end-users.
- **Request:** (External) End-user-facing term for standard work items (e.g., "I need a loaner laptop").
- **Incident:** (Both) A specialized work item meaning "something is broken." Fast SLA clock.
- **Task:** (Internal Only) Child work item. Discrete piece of work assigned to a queue to fulfill a parent request.

### People Roles on a Ticket
- **Submitter:** The person who physically typed and sent the request (e.g. an executive assistant or agent).
- **Requester / Beneficiary:** The person the work is being done FOR. Receives all emails and tracking links.
- **Assignee:** The individual IT/HR agent responsible for resolving the work.
- **Watcher:** A person who has read-only visibility for updates (no assigned action).

### Display ID Convention
Format: `{ORG_PREFIX}-{TYPE_PREFIX}-{SEQUENCE}`
- `ACME-REQ-0042` (Request)
- `ACME-INC-0017` (Incident)
- `ACME-TASK-0089` (Task)


---

## 3. Logical Data Model (Part A: New Tables)

### 3.1 `departments` Table
Represents the top-level corporate structure of a customer organization (e.g. "IT", "HR").

| Column | Type | Purpose |
|---|---|---|
| `id` | UUID | Unique identifier |
| `org_id` | UUID | Which customer company owns this |
| `name` | TEXT | e.g. "Human Resources" |
| `description` | TEXT | Free-text detail |
| `head_user_id` | UUID | Department head (e.g. the CIO) |
| `active` | BOOLEAN | Soft-deactivation switch |
| `created_at` | TIMESTAMPTZ | Audit |
| `updated_at` | TIMESTAMPTZ | Audit |

**Constraint:** `UNIQUE(org_id, name)` prevents duplicate departments.

### 3.2 `department_queues` Table (Junction / Map)
Connects queues to departments. Allows a single queue (like "Security") to serve multiple departments (like "IT" and "Legal").

| Column | Type | Purpose |
|---|---|---|
| `department_id` | UUID | The related department |
| `queue_id` | UUID | The related queue |
| `relationship` | TEXT | 'owner', 'consumer', or 'observer' |
| `created_at` | TIMESTAMPTZ | Audit |

**Cascade Behavior:**
- Deleting a department is BLOCKED (RESTRICT) if queues are still attached (forces clean migration).
- Deleting a queue automatically removes its junction rows (CASCADE).


### 3.3 `routing_rules` Table
The deterministic keyword matcher that routes tickets before AI intervention.

| Column | Type | Purpose |
|---|---|---|
| `id` | UUID | Unique identifier |
| `org_id` | UUID | Tenant isolation |
| `queue_id` | UUID | Destination queue |
| `name` | TEXT | e.g. "Route Okta to IAM" |
| `priority` | INT | Order of execution (lower runs first, e.g. 10 before 20) |
| `match_type` | TEXT | 'keyword' (v1), 'regex', 'domain' |
| `match_field`| TEXT | 'subject', 'body', 'subject_or_body' |
| `match_value`| TEXT | String to look for (e.g. "okta", "laptop") |
| `case_sensitive` | BOOLEAN | Default false |
| `active` | BOOLEAN | Enable / disable rule |
| `hit_count` | INT | Number of times this rule matched |
| `last_matched_at` | TIMESTAMPTZ | Audit / tracking |

### 3.4 Extensions to `tickets` Table
Extends existing `tickets` to support work item types, parent-child tasks, and OBO (on-behalf-of) ownership.

- `type` (TEXT, default `'request'`): `'request'`, `'incident'`, `'task'`, `'change'`
- `parent_ticket_id` (UUID, nullable): Points to parent ticket if this is a child task
- `is_parent` (BOOLEAN, default `false`): True if this ticket has child tasks
- `child_count` (INT, default 0): Total number of child tasks
- `child_resolved_count` (INT, default 0): Number of completed child tasks
- `submitted_by_customer_id` (UUID, nullable): Who typed/emailed the request
- `submitted_by_user_id` (UUID, nullable): Agent who filed on behalf of someone
- `requester_customer_id` (UUID, nullable): The person the work is for (gets all emails)


### 3.5 `ticket_watchers` Table
Tracks agents or customers who want visibility into a work item without being the assignee.

| Column | Type | Purpose |
|---|---|---|
| `ticket_id` | UUID | Which ticket |
| `user_id` | UUID | Agent watcher (nullable) |
| `customer_id` | UUID | Customer watcher (nullable) |
| `reason` | TEXT | 'requester', 'manager', 'stakeholder', 'auto_added' |
| `created_at` | TIMESTAMPTZ | Audit |

### 3.6 Extensions to `comments` Table (Department Privacy)
Controls which department can see internal comments (e.g. HR notes hidden from IT).

- `visibility_scope` (TEXT, default `'internal'`): `'public'`, `'internal'`, `'department_only'`
- `visible_to_dept_id` (UUID, nullable): Points to `departments.id` if scoped to a specific department

### 3.7 `ticket_type_prefixes` Table
Configurable display prefixes per organization and work item type.

| Column | Type | Purpose |
|---|---|---|
| `org_id` | UUID | Tenant isolation |
| `type` | TEXT | 'request', 'incident', 'task', 'change' |
| `prefix` | TEXT | e.g. "REQ", "INC", "TASK", "CHG" |
| `created_at` | TIMESTAMPTZ | Audit |


---

## 4. State Machines & Behavioral Specifications

### 4.1 Submitter vs. Requester Flow
- **Direct Submission:** When an email arrives, `submitted_by_customer_id = requester_customer_id`.
- **On-Behalf-Of Submission:** When an agent files for someone, `submitted_by_user_id` is the agent, while `requester_customer_id` is the employee.
- **Notification Rule:** All outbound emails, confirmation links, and surveys MUST be addressed to `requester_customer_id`.

### 4.2 Parent-Child Grace Period State Machine
When all child tasks under a parent request are completed:
1. The parent enters a **24-hour grace period** (marked resolved).
2. If the requester emails back within 24 hours with an issue, the parent and tasks automatically reopen.
3. If 24 hours pass with no replies, the parent transitions to `closed` (immutable).

### 4.3 The Routing Waterfall
Every inbound work item is evaluated in this exact order:
1. **Deterministic Rules:** Query `routing_rules` by priority. If a keyword matches, assign queue immediately (0ms, $0 cost).
2. **AI Classification:** If no rule matches, invoke AI model to analyze context and suggest queue.
3. **Triage Queue Fallback:** If AI confidence < 0.70 (or AI is unavailable), route to the default "Triage" queue for human review.


---

## 5. Explicit Non-Goals (v1)

1. **Regex Routing Rules:** v1 supports exact/substring keyword matching only.
2. **Domain-Based Routing:** Keyword matching first; domain routing deferred.
3. **Delegated Dept-Head Authority:** v1: Org admins manage all departments/queues/rules.
4. **Nested Departments:** Flat, single-level departments only.
5. **Nested Parent-Child Tasks (Grandchildren):** Parents can have children, but children cannot have children.
6. **CAB & Change Management Workflows:** Schema exists, but workflows deferred.
7. **Problem Management:** No dedicated problem workflow in v1.

---

## 6. Migration Sequencing

Due to foreign key dependencies, migrations must execute in this exact order:

1. `create_departments_table` (depends on: organizations, users)
2. `create_department_queues_table` (depends on: departments, queues)
3. `alter_tickets_add_ownership` (depends on: tickets, customers, users)
4. `alter_tickets_add_parent_child` (depends on: tickets self-reference)
5. `create_ticket_watchers_table` (depends on: tickets, users, customers)
6. `alter_comments_add_visibility` (depends on: comments, departments)
7. `create_routing_rules_table` (depends on: organizations, queues, users)
8. `create_ticket_type_prefixes_table` (depends on: organizations)


---

## 7. Change Log

| Date | Version | Summary |
|---|---|---|
| 2025 | 1.0 | Initial architecture & routing topology ratified |

---
**END OF DOCUMENT**

# Credit System for Goop2

## Vision

Goop2 is free to run, free to connect peers directly, and free to use basic templates. The credit system introduces a virtual currency ("credits") that gates access to premium templates distributed through rendezvous servers. This is the monetization layer.

**Free forever:**

- Running a peer node
- Direct P2P connections (no rendezvous)
- Core functionality (chat, file sharing, groups, realtime channels)
- Basic/bundled templates (chess, corkboard, etc.)

**Costs credits:**

- Premium templates published on a rendezvous server
- Potentially: extra storage quotas, priority relay, cosmetic features (future)

Credits are purchased with real money (eventually), but the system ships first with starter credits so the mechanics can be tested without payment infrastructure.

---

## Architecture

### Why Rendezvous is the Ledger

The rendezvous server is the natural authority for credits:

1. **Already trusted** — peers register there, discover each other through it, and download templates from it
2. **Centralized by design** — each rendezvous is an independent server with its own economy
3. **Tamper-resistant** — peers can't edit a server-side database to give themselves credits
4. **Auditable** — all transactions are logged server-side

A peer's local SQLite is NOT involved in credit accounting. The peer may cache their balance for display purposes, but the rendezvous is always the source of truth.

### Multiple Rendezvous, Multiple Economies

Each rendezvous server runs its own independent credit economy. Credits on Server A have no relation to credits on Server B. This is intentional:

- Different rendezvous operators can set their own pricing
- No need for cross-server settlement
- Keeps the system simple and self-contained
- Each rendezvous operator is essentially running their own "app store"

### Relationship to Open Source

The credit system is a **rendezvous-server feature**, not a core protocol feature. The goop2 peer code remains fully functional without credits. Anyone can:

- Run their own rendezvous server with credits disabled (all templates free)
- Fork and modify the credit logic
- Build alternative monetization on top of the same protocol

The commercial value is in operating a rendezvous server with curated, quality templates and a working credit economy — not in locking down the peer software.

---

## Template Pricing

### Manifest Changes

The template `manifest.json` gains a `price` field:

```json
{
  "name": "chess",
  "title": "Chess",
  "description": "Play chess against friends or AI",
  "version": "1.0.0",
  "author": "goop2",
  "price": 0
}
```

```json
{
  "name": "poker",
  "title": "Texas Hold'em",
  "description": "Multiplayer poker with real-time bidding",
  "version": "1.0.0",
  "author": "goop2",
  "price": 100
}
```

- `price: 0` — free, anyone can install
- `price: N` (N > 0) — costs N credits to install

### Pricing Tiers (Suggested)

| Tier | Credits | Template Examples |
| -- | -- | -- |
| Free | 0 | Chess, corkboard, photobook, quiz |
| Basic | 50 | Simple games, utility apps |
| Standard | 100-200 | Multiplayer games, productivity tools |
| Premium | 500+ | Complex apps, specialized tools |

The rendezvous operator decides which templates are free and which cost credits. The manifest `price` is set when the template is published to the rendezvous.

### One-Time Purchase vs Subscription

**Phase 1: One-time purchase.** You pay once, you own the template forever. Simplest model, easiest to understand.

**Future consideration:** A subscription model (credits/month for access to all templates) could work but adds significant complexity. Not worth building until the one-time model is proven.

---

## Data Model

### Rendezvous Database Tables

```sql
-- Credit balances (one row per registered peer)
CREATE TABLE credit_accounts (
    peer_id     TEXT PRIMARY KEY,
    balance     INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Transaction log (append-only, never modified)
CREATE TABLE credit_transactions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    peer_id     TEXT NOT NULL,
    amount      INTEGER NOT NULL,        -- positive = credit, negative = debit
    balance     INTEGER NOT NULL,        -- balance AFTER this transaction
    type        TEXT NOT NULL,           -- 'starter', 'purchase', 'refund', 'grant', 'buy'
    ref_type    TEXT,                    -- 'template', 'bundle', etc.
    ref_id      TEXT,                    -- template name, bundle ID, etc.
    note        TEXT,                    -- human-readable description
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,

    FOREIGN KEY (peer_id) REFERENCES credit_accounts(peer_id)
);
CREATE INDEX idx_txn_peer ON credit_transactions(peer_id, created_at DESC);

-- Template purchases (tracks which peer owns which template)
CREATE TABLE template_purchases (
    peer_id       TEXT NOT NULL,
    template_name TEXT NOT NULL,
    price_paid    INTEGER NOT NULL,
    txn_id        INTEGER NOT NULL,      -- FK to credit_transactions.id
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (peer_id, template_name),
    FOREIGN KEY (peer_id) REFERENCES credit_accounts(peer_id),
    FOREIGN KEY (txn_id) REFERENCES credit_transactions(id)
);
```

### Transaction Types

| Type | Amount | Description |
| -- | -- | -- |
| `starter` | +N | Initial free credits on registration |
| `purchase` | -N | Bought a template |
| `refund` | +N | Template refund (admin action) |
| `grant` | +N | Admin grants credits (promotions, testing) |
| `buy` | +N | Purchased credits with real money (future) |

### Why an Append-Only Transaction Log?

- Complete audit trail — can reconstruct any balance from scratch
- Dispute resolution — can see exactly what happened
- Analytics — understand spending patterns
- The `balance` column on each row is a running total, so you never need to sum all transactions to get current balance (but you CAN verify it)

---

## API Design

All credit endpoints live on the rendezvous server under `/api/credits/`.

### Authentication

Credit API calls are authenticated by peer ID. The rendezvous already knows which peer is making the request (from the registration/session mechanism). No separate auth token is needed for Phase 1.

**Future:** When real money is involved, add proper authentication (signed requests, API keys, or session tokens).

### Endpoints

#### `GET /api/credits/balance`

Returns the caller's current credit balance.

```json
// Response
{
  "balance": 450,
  "peer_id": "12D3Koo..."
}
```

#### `GET /api/credits/transactions?limit=20&offset=0`

Returns the caller's transaction history.

```json
// Response
{
  "transactions": [
    {
      "id": 42,
      "amount": -100,
      "balance": 450,
      "type": "purchase",
      "ref_type": "template",
      "ref_id": "poker",
      "note": "Purchased template: Texas Hold'em",
      "created_at": "2026-02-07T10:30:00Z"
    },
    {
      "id": 1,
      "amount": 500,
      "balance": 550,
      "type": "starter",
      "note": "Welcome bonus",
      "created_at": "2026-02-07T09:00:00Z"
    }
  ],
  "total": 2
}
```

#### `POST /api/credits/purchase`

Purchase (unlock) a template. Deducts credits and records the purchase.

```json
// Request
{
  "template": "poker"
}

// Response (success)
{
  "status": "ok",
  "template": "poker",
  "price": 100,
  "new_balance": 450
}

// Response (insufficient credits)
{
  "error": "insufficient_credits",
  "balance": 50,
  "price": 100,
  "shortfall": 50
}

// Response (already owned)
{
  "status": "already_owned",
  "template": "poker"
}
```

#### `GET /api/credits/purchases`

Returns the list of templates the caller has purchased.

```json
// Response
{
  "purchases": [
    {
      "template": "poker",
      "price_paid": 100,
      "purchased_at": "2026-02-07T10:30:00Z"
    }
  ]
}
```

#### `POST /api/credits/grant` (Admin only)

Grant credits to a peer. Used for testing, promotions, customer support.

```json
// Request
{
  "peer_id": "12D3Koo...",
  "amount": 500,
  "note": "Beta tester bonus"
}

// Response
{
  "status": "ok",
  "new_balance": 950
}
```

---

## Client-Side Flows

### 1. Registration + Starter Credits

```bash
Peer registers on rendezvous
        ↓
Rendezvous creates credit_account with balance = STARTER_CREDITS (e.g., 500)
        ↓
Logs a 'starter' transaction
        ↓
Client receives balance in registration response
```

The starter credit amount is configurable per rendezvous server. Suggested default: **500 credits** — enough to try 2-3 premium templates.

### 2. Browsing Templates

When the peer browses the template catalog on the rendezvous:

```bash
GET /api/templates/list
        ↓
Response includes price + owned status for each template:
[
  { "name": "chess",  "title": "Chess",       "price": 0,   "owned": true  },
  { "name": "poker",  "title": "Hold'em",     "price": 100, "owned": false },
  { "name": "kanban", "title": "Kanban",      "price": 0,   "owned": true  }
]
```

The UI shows:

- Free templates: "Install" button (always available)
- Owned premium templates: "Install" button (already purchased)
- Unowned premium templates: price tag + "Buy for 100 credits" button
- Balance displayed in header/navbar

### 3. Purchasing a Template

```bash
User clicks "Buy for 100 credits" on a template
        ↓
Client shows confirmation dialog:
  "Purchase 'Texas Hold'em' for 100 credits?
   Your balance: 450 credits → 350 credits"
        ↓
User confirms
        ↓
POST /api/credits/purchase { "template": "poker" }
        ↓
Rendezvous (in one transaction):
  1. Check balance >= price
  2. Deduct credits
  3. Record transaction
  4. Record purchase
        ↓
Response: { status: "ok", new_balance: 350 }
        ↓
Client updates balance display
        ↓
Template is now available for download/install
```

### 4. Installing a Purchased Template

After purchase, the peer downloads and installs the template. The install flow is the same as today — the purchase just gates access.

The rendezvous `/api/templates/download` endpoint should check `template_purchases` before serving paid template files. Free templates (price = 0) are served without a purchase check.

### 5. Balance Display

The credit balance should be visible in the rendezvous UI:

- **Navbar:** Small credit balance indicator (e.g., "450 credits" or a coin icon + number)
- **Template catalog:** Each template shows its price
- **Template detail:** Shows price, "Buy" button, or "Owned" badge
- **Settings/account page:** Full transaction history

---

## Template Access Control

### Who Can Install What?

| Template Price | Peer Status | Can Install? |
| -- | -- | -- |
| Free (0) | Any | Yes |
| Paid (N > 0) | Has purchased | Yes |
| Paid (N > 0) | Has not purchased | No — must buy first |
| Paid (N > 0) | Sufficient balance | Can buy, then install |
| Paid (N > 0) | Insufficient balance | Cannot buy — needs more credits |

### What About Already-Installed Templates?

If a template was installed before the credit system existed, the peer keeps it. The credit system only applies to NEW installations going forward. Existing installs are grandfathered.

### What About Template Updates?

Once purchased, the peer can update the template for free. A purchase grants permanent access to that template including all future versions.

---

## Security Considerations

### Preventing Double-Spend

The purchase endpoint uses a database transaction:

```sql
BEGIN;
  SELECT balance FROM credit_accounts WHERE peer_id = ? FOR UPDATE;
  -- check balance >= price
  UPDATE credit_accounts SET balance = balance - ?, updated_at = CURRENT_TIMESTAMP WHERE peer_id = ?;
  INSERT INTO credit_transactions (...) VALUES (...);
  INSERT INTO template_purchases (...) VALUES (...);
COMMIT;
```

SQLite doesn't support `FOR UPDATE`, but since the rendezvous is single-process with WAL mode, wrapping in a single `BEGIN IMMEDIATE` transaction prevents races.

### Preventing Credit Forgery

Credits only exist on the rendezvous server. Peers cannot create, modify, or transfer credits. The only ways credits enter the system are:

1. Starter bonus (on registration)
2. Admin grant
3. Real-money purchase (future)

### Preventing Unauthorized Template Access

The template download endpoint must verify ownership:

```bash
GET /api/templates/download?name=poker
        ↓
Check: Is this template free (price=0)?
  → Yes: serve files
  → No: Does this peer have a record in template_purchases?
    → Yes: serve files
    → No: return 402 Payment Required
```

### Rate Limiting

All credit endpoints should be rate-limited to prevent abuse:

- Balance check: 60/min
- Purchase: 10/min
- Transaction history: 30/min

---

## Onboarding & First-Time Experience

### New Peer Registers on Rendezvous

1. Peer connects to rendezvous for the first time
2. Rendezvous creates account + grants starter credits (500)
3. Client shows welcome message: "Welcome! You've received 500 credits to get started."
4. Template catalog highlights a few recommended premium templates
5. User can browse, try free templates, and spend credits on premium ones

### Running Low on Credits

When balance drops below a threshold (e.g., 100), the UI can show a subtle nudge:

- "Running low on credits. [Get more]"
- Link goes to the "buy credits" page (future: Stripe checkout)
- For now (Phase 1): "Contact the server admin for more credits"

---

## Future: Real-Money Purchases

### Phase 2: Credit Bundles

Once the credit mechanics are tested and working, add real-money purchases:

```bash
Credit Bundles:
  - 500 credits  → $4.99
  - 1200 credits → $9.99  (20% bonus)
  - 3000 credits → $19.99 (50% bonus)
```

### Payment Integration Options

| Provider | Pros | Cons |
| -- | -- | -- |
| **Stripe** | Industry standard, great API, handles tax | Monthly fees, complex for small projects |
| **Lemon Squeezy** | Built for digital products, handles tax globally | Less known, smaller ecosystem |
| **Ko-fi / Buy Me a Coffee** | Zero setup, no monthly fees | Less professional, limited control |
| **Direct crypto** | No middleman, P2P aligned | Volatile, regulatory complexity, UX friction |

**Recommended for Phase 2:** Stripe or Lemon Squeezy. Both handle tax compliance and have good developer APIs.

### Payment Flow (Future)

```bash
User clicks "Buy 500 Credits for $4.99"
        ↓
Redirect to Stripe Checkout (or embedded form)
        ↓
User pays
        ↓
Stripe webhook → rendezvous server
        ↓
Rendezvous credits the account + logs 'buy' transaction
        ↓
User returns to app, balance updated
```

---

## Peer-to-Peer Credit Transfer (NOT Recommended for Phase 1)

Allowing peers to send credits to each other introduces significant complexity:

- **Fraud risk:** Stolen accounts could drain credits
- **Money transmission laws:** In many jurisdictions, allowing user-to-user transfers of purchased currency triggers financial regulations
- **Complexity:** Requires transfer limits, reversal mechanisms, dispute resolution

**Recommendation:** Do NOT implement P2P credit transfer in Phase 1 (or possibly ever). Keep credits as a one-way flow: buy credits → spend on templates.

---

## Implementation Phases

### Phase 1: Core Credit System (Build & Test)

- [ ] Add `price` field to template manifest
- [ ] Create credit tables on rendezvous DB
- [ ] Implement credit API endpoints (balance, purchase, transactions)
- [ ] Grant starter credits on registration
- [ ] Gate template downloads by purchase status
- [ ] Client UI: balance display, price tags, purchase flow
- [ ] Admin grant endpoint for testing

### Phase 2: Payment Integration

- [ ] Stripe/Lemon Squeezy integration
- [ ] Credit bundle purchase page
- [ ] Webhook handler for payment confirmation
- [ ] Receipt generation

### Phase 3: Polish & Analytics

- [ ] Spending analytics dashboard (for rendezvous operators)
- [ ] Low-balance notifications
- [ ] Promotional credit campaigns
- [ ] Referral bonuses (invite a peer, both get credits)

---

## Open Questions

1. **Starter credit amount:** 500? 1000? Too many = no urgency to buy. Too few = frustrating first experience.

2. **Refund policy:** Allow refunds within N hours of purchase? Or no refunds (keep it simple)?

3. **Template trials:** Allow a "preview" of premium templates before purchase? (e.g., read-only demo mode)

4. **Rendezvous operator revenue share:** If third parties publish templates on your rendezvous, do they get a cut of the credits spent? (This is an app-store model and adds significant complexity.)

5. **Credit expiry:** Do credits expire? Probably not — expiring credits feel punitive and create bad UX. Keep them permanent.

6. **Multiple rendezvous accounts:** A peer can register on multiple rendezvous servers. Each has its own separate balance. Is this clear enough to users?

---

## Summary

The credit system turns the rendezvous server into a lightweight app store. Free templates remain free. Premium templates cost credits. Credits are managed entirely server-side with a clean API. Phase 1 uses starter credits (no real money), Phase 2 adds payment integration.

The key design principle: **the P2P core stays free and open; monetization lives at the rendezvous layer.**

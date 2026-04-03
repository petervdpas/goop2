# goop2-services Architecture

## Overview

<!-- STUB: Separate repo: ~/Projects/goop2-services/ -->
<!-- Monorepo with independent services: credits/, registrations/, email/, templates/, bridge/, encryption/ -->
<!-- Each service is a standalone binary with its own HTTP server -->
<!-- Zero imports between repos — goop2 gateway proxies HTTP calls -->

## Services

<!-- STUB: -->
<!-- credits/ (:8800) — credit balances, template pricing, purchases. Tied to registered email accounts. -->
<!-- registrations/ (:8801) — user registration, email verification -->
<!-- email/ (:8802) — SMTP sending, HTML email templates, verification emails -->
<!-- templates/ (:8803) — template store, bundling, pricing, manifest parsing -->
<!-- bridge/ — WebSocket bridge for browser-only peers (no libp2p) -->
<!-- encryption/ — NaCl key exchange, broadcast key distribution -->

## Dependency chain

<!-- STUB: registration → credits ↔ templates -->
<!-- registration calls email for verification -->
<!-- credits proxies price lookups to templates -->
<!-- bridge and encryption are independent -->

## Gateway proxy

<!-- STUB: How goop2 proxies to services -->
<!-- Config: credits_url, registration_url, email_url, templates_url -->
<!-- RemoteCreditProvider in rendezvous package proxies credit operations -->
<!-- Template store fetches from templates service when templates_url is set -->

## Credit flow

<!-- STUB: viewer → gateway → credits service -->
<!-- Spend flow for premium templates -->
<!-- Balance checks, access control -->

## Shared types

<!-- STUB: StoreMeta / TablePolicy defined in both repos -->
<!-- template_meta.go in both goop2/internal/rendezvous/ and goop2-services/templates/ -->
<!-- Must be kept in sync manually (same JSON shape) -->

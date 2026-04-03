# Viewer & HTTP Layer

## Deps struct

<!-- STUB: Central dependency container passed to all route registrations -->
<!-- Key fields: Node, DB, GroupManager, TemplateHandler, Content, MQ, EnsureLua, LuaCall -->
<!-- Assembled in viewer.go from the Viewer struct, which is assembled in peer.go -->

## Route registration

<!-- STUB: handleGet/handlePost helpers with JSON decode -->
<!-- CSRF token flow: generated on Register(), passed to templates -->
<!-- All routes registered in register.go via Register(mux, deps) -->
<!-- Separate registration functions per domain: RegisterGroups, registerTemplateRoutes, etc. -->

## Template rendering

<!-- STUB: Go html/template with render.InitTemplates() -->
<!-- Templates in internal/ui/templates/*.html -->
<!-- Layout system: base.html wraps page templates -->

## UI assets loading

<!-- STUB: app.js loads all JS files sequentially -->
<!-- Three groups: sharedFiles (core, api, mq, select, panel, ...) → callFiles → pageFiles -->
<!-- Each page JS file self-initializes by checking for its DOM elements -->

## Admin UI vs SDK

<!-- STUB: Two separate JS ecosystems in the same app -->
<!-- Admin UI: internal/ui/assets/js/ — core.js, groups.js, panel.js, pages/*.js -->
<!-- Uses Goop.core, Goop.api, Goop.panel, Goop.groups, Goop.dialog -->
<!-- SDK: internal/sdk/goop-*.js — served at /sdk/ for template viewers -->
<!-- Uses Goop.data, Goop.ui, Goop.group, Goop.template, etc. -->
<!-- SDK is fully standalone — no dependency on admin UI core.js -->

## OpenAPI annotations

<!-- STUB: Swagger docs generated from openapi_annotations.go -->
<!-- Every HTTP endpoint has a matching swagger stub function -->
<!-- Request/response types defined as unexported structs in the same file -->

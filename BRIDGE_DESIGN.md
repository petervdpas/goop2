# HTTPS Bridge Design Documentation

All design documentation has been moved to the **`bridge-design/`** folder.

## Quick Start

**Start here**: [`bridge-design/README.md`](./bridge-design/README.md)

---

## Documents

| Document | Purpose | Audience |
|----------|---------|----------|
| **[PLUGIN_ARCHITECTURE_CLARIFICATION.md](./bridge-design/PLUGIN_ARCHITECTURE_CLARIFICATION.md)** | How bridge fits into goop2-services (static config, not dynamic) | Everyone |
| **[HTTPS_vs_HTTP_INTERNAL.md](./bridge-design/HTTPS_vs_HTTP_INTERNAL.md)** | Why external is HTTPS, internal is HTTP (detailed security analysis) | Security, DevOps |
| **[MICROSERVICE_ASSESSMENT.md](./bridge-design/MICROSERVICE_ASSESSMENT.md)** | Executive summary (viability, effort, trade-offs) | Managers, Architects |
| **[SWAGGER_SDK_GENERATION.md](./bridge-design/SWAGGER_SDK_GENERATION.md)** | Auto-generate client SDKs from OpenAPI spec (industry standard) | Developers |
| **[HTTPS_BRIDGE_DESIGN.md](./bridge-design/HTTPS_BRIDGE_DESIGN.md)** | Complete technical specification (85KB, code examples, diagrams) | Developers |
| **[DESIGN_DOCS_INDEX.md](./bridge-design/DESIGN_DOCS_INDEX.md)** | Navigation guide and cross-document index | Everyone |

---

## Status

✅ **Complete** - Ready for implementation

---

## Key Facts

- **Microservice?** ✅ YES
- **Plugin pattern?** ✅ Static config (like other services)
- **External protocol?** HTTPS (TLS 1.3)
- **Internal protocol?** HTTP (trusted network)
- **Implementation?** ~1,600 LOC, ~10 days
- **Scalable?** ✅ Multiple instances possible

---

**→ [Start reading: bridge-design/README.md](./bridge-design/README.md)**

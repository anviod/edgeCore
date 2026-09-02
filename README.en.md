# Industrial Edge Gateway

[中文 README](README.md) | [Documentation Site](https://anviod.github.io/edgeCore/) | [GPL-3.0](LICENSE)

Industrial Edge Gateway is a lightweight industrial edge computing gateway for manufacturing, energy, and building automation. It bridges **OT devices ↔ IT systems** in a single gateway — unified southbound access, local edge processing, and flexible northbound integration from acquisition to reporting — with MCP support for LLM integration and industrial-grade SLA for long-term reliability.

<div align="center">
  <img src="./docs/img/dataScanEngineCN.svg" width="100%" alt="Data flow overview" />
  <p><small>Data flow overview</small></p>
</div>

## Core Capabilities

| Capability | Value |
| :--- | :--- |
| **Reliability** | industrial-grade SLA: metric gates · Soak regression · CI five-gate, 10k-tag benchmark stable with no memory leak |
| **Southbound** | 13 industrial protocols, unified OT collection: device discovery · object scan · batch tag registration |
| **Scheduling** | 10ms-class kernel · in-memory shadow as source of truth, P99 lag <150ms (≤10k tags) |
| **Edge** | rule engine · virtual shadow derived compute: cross-device mapping · formula aggregation · local control linkage |
| **Northbound** | MQTT · Sparkplug B · OPC UA · BACnet · EdgeOS |

Lightweight deployment: single binary · static build · no runtime dependencies, min 128MB RAM, x86_64 / ARMv7 / ARM64.

## Quick Start

```bash
go mod tidy && go run cmd/main.go              # backend, default -conf ./conf
cd ui && npm install && npm run build && cd .. # frontend (served from ui/dist)

# One-command multi-platform packaging (GoReleaser, output in ./dist/)
goreleaser release --snapshot --clean
```

Install/upgrade via deb / rpm and systemd: see [User Manual](https://anviod.github.io/edgeCore/guide/USER_MANUAL.html).

## Documentation

| Document | Description |
| :--- | :--- |
| [Product Guide](https://anviod.github.io/edgeCore/guide/%E4%BA%A7%E5%93%81%E8%AF%B4%E6%98%8E.html) (中文) | capabilities · SLA metrics · changelog |
| [User Manual](https://anviod.github.io/edgeCore/guide/USER_MANUAL.html) | protocols · deployment · operations · best practices |
| [Architecture](https://anviod.github.io/edgeCore/architecture/index.html) | ScanEngine kernel · ShadowCore · system design |
| [Driver Matrix](https://anviod.github.io/edgeCore/drivers/index.html) | 13 protocol drivers and development standards |
| [MCP Guide](https://anviod.github.io/edgeCore/guide/mcp-access-guide.html) | MCP integration, tool inventory, client setup |
| [Documentation Site](https://anviod.github.io/edgeCore/) | full docs and roadmap |

## License

This project is licensed under the [GNU General Public License v3.0](LICENSE).
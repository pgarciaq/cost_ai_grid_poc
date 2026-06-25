# Cost Management — AI Grid PoC

A proof-of-concept integrating [Red Hat Cost Management](https://github.com/project-koku/koku) with [OSAC](https://github.com/osac-project) (Open Sovereign AI Console) for the AI Grid sovereign cloud offering.

---

## What it does

- Ingests CloudEvents from OSAC for resource lifecycle (clusters, VMs, models, bare metal)
- Meters capacity-based resources (CaaS, VMaaS) and consumption-based resources (MaaS — tokens/requests)
- Tracks budgets and quotas, emitting threshold alerts back to OSAC
- Exposes a FastAPI REST API for cost, metering, inventory, and quota data

## Architecture

See [`docs/poc_architecture/architecture.md`](docs/poc_architecture/architecture.md).

## Stack

| Layer | Choice |
|---|---|
| Language | Python (uv + pyproject.toml) |
| API | FastAPI |
| Storage | PostgreSQL |
| ORM | SQLAlchemy + Alembic |
| Event format | CloudEvents 1.0 |
| Event transport | Kafka (KRaft) |

## Docs

- [`docs/poc_architecture/architecture.md`](docs/poc_architecture/architecture.md) — system design and component map
- [`docs/poc_architecture/data-model.md`](docs/poc_architecture/data-model.md) — database schema
- [`docs/poc_architecture/event-types.md`](docs/poc_architecture/event-types.md) — CloudEvents reference
- [`docs/poc_architecture/Metering/koku_cost_model_summary.md`](docs/poc_architecture/Metering/koku_cost_model_summary.md) — Koku OCP cost model metrics reference
- [`docs/requirements/ai_grid_poc_requirements_brief.md`](docs/requirements/ai_grid_poc_requirements_brief.md) — requirements and action items
- [`docs/requirements/csv_poc_requirements_summary.md`](docs/requirements/csv_poc_requirements_summary.md) — cost management requirements summary
- [`docs/development/fullfillment_service_setup.md`](docs/development/fullfillment_service_setup.md) — local dev setup

## License
Discovery artifacts and scripts in this repository are part of the [Koku](https://github.com/project-koku/koku) project. OSAC is a separate open-source project with its own license — see the OSAC repository for details.

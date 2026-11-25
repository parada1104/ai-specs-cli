# demo-change

Un plan completo y válido: tres requisitos, tres tareas, un log y la verificación por `S#`.

Card: `000000000000000000000000` — [demo](https://trello.com/c/demo)

## Specs

- **S1.** El gate exige el plan y no un `tasks.md` libre — *"el plan es el contrato de entrega"*
- **S2.** Cada fila de tareas cita el `S#` que implementa — *"tasks enlazadas a S#"*
- **S3.** El log guarda la petición del humano verbatim — *"L1 es la petición original"*

## Tasks

| ID | Spec | Tarea | Ruta | Commit |
| --- | --- | --- | --- | --- |
| T1 | S1 | Parser del plan con un test por regla | worker | `aaaaaaa` |
| T2 | S1, S2 | Cablear el parser en el gate de cada tier | worker | pendiente |
| T3 | — | Cerrar la revisión con el log y el veredicto | inline | pendiente |

## Design

Una sola decisión: el parser vive en `internal/gates` porque el gate es su único consumidor.

## Verification

- **S1: PASS** — `go test ./internal/gates/...`, exit 0, commit `aaaaaaa`, 2026-01-01
- **S2: PASS** — `go vet ./internal/gates/...`, exit 0, commit `aaaaaaa`, 2026-01-01
- **S3: N/A** — la redacción verbatim es prosa y el CLI no la puede juzgar

## Log

- **L1** — Petición original: *"implementa I2 con un test por regla"*

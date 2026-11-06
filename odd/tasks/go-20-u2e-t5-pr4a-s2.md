# GO-20 U2e T5 PR4a-S2

Base: `172d66581d24fcc5def01d8d89b294569cb31d89`; tracker: https://trello.com/c/OuodQSGe (card `6adcb0a6f1eed7e544dcaf5e`).
Delivery: bounded work units against `epic/go-single-binary`; Python bridge tests remain untouched.

## Specs

- S1: «congelar únicamente los oráculos Go `stamps_test.go` y `orphans_test.go`; las pruebas de puente Python quedan para un slice posterior».
- S2: «Implementa solo ese alcance, en candidatos simples y acotados; no incluyas ni retires pruebas puente ni amplíes a otras familias.»
- S3: «Sigue el contrato del repo y conduce la entrega normal autorizada de esta migración».

## Tasks

- T1 (S1–S2): Freeze the stamps oracle, preserve assertions, run RED/GREEN and focused checks. Route: inline; commit: this work unit.
- T2 (S1–S2): Freeze the orphans oracle, preserve assertions, run RED/GREEN and focused checks. Route: inline; commit: pending.
- T3 (S3): Run full validation and applicable RDD assessment; deliver under repository gates. Route: parent; commit: pending.

## Log

- Scratch: `/Users/robert/.hermes/profiles/cody/cache/scratch/go20-pr4a-s2` (physical path verified); all subsequent test/build commands must set `TMPDIR` explicitly to this path. Earlier stamps capture and RED/GREEN lacked explicit `TMPDIR`; their harness logs are retained outside scratch. An incomplete validation job was stopped before continuation and is not validation evidence.
- Stamps validation job-6: exit 1; `go vet ./internal/sync` and `go test ./internal/sync -count=1` passed; full `tests/validate.sh` failed in `TestUpgradeUsageMatchesTheShellSurface` because `dist/ai-specs` was absent. Preserved in `stamps-validation.log` under scratch; not a passing full validation.
- Stamps: captured live Python 3.14.7 oracle; RED missing golden, GREEN frozen oracle. After a plain `go build -o dist/ai-specs ./cmd/ai-specs` (tracked diff hash unchanged), upgrade usage focused test passed. Validation job-8 exited 0: 2345 tests, 165 skipped, 3 expected failures; warnings report unavailable verified gate and fallback. Log `stamps-validation-after-build.log` in scratch; skips and warning are not stronger gate-present evidence.
- L1: Robert confirmó el alcance de PR4a-S2: congelar únicamente los oráculos Go `stamps_test.go` y `orphans_test.go`; las pruebas de puente Python quedan para un slice posterior. Continúa el change existente: recupera Engram y el documento de trabajo, reconcilia el siguiente candidato RDD y el worktree/base correctos con Git/PR antes de editar. Implementa solo ese alcance, en candidatos simples y acotados; no incluyas ni retires pruebas puente ni amplíes a otras familias. Sigue el contrato del repo y conduce la entrega normal autorizada de esta migración; escala únicamente si aparece una decisión de producto/permiso genuinamente pendiente o un bloqueo que cambie el alcance. No vuelvas a pedir confirmación de S2.

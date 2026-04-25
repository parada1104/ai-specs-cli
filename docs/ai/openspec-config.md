# OpenSpec Config

## Purpose

`openspec/config.yaml` is the project-local OpenSpec configuration. It should be
valid for the installed `spec-driven` schema so normal SDD commands do not emit
configuration warnings.

## Supported Shape

The `rules` map should only contain schema artifacts, and each artifact value
should be a list of strings. For this repository that means:

- `rules.proposal`
- `rules.specs`
- `rules.design`
- `rules.tasks`
- `rules.apply`
- `rules.verify`

Do not put nested objects under `rules.apply` or `rules.verify`. Do not add
`rules.archive` while `archive` is not an artifact in the installed schema.

## Project Guidance

Richer project-specific metadata that does not fit the OpenSpec `rules` shape
lives under `ai_specs_guidance`. That section is for humans and local tooling;
it is not part of the OpenSpec artifact rules contract.

## Validation

The final project verification command is `./tests/validate.sh`. During apply,
`./tests/run.sh` can be used for faster feedback when only the unittest suite is
needed.

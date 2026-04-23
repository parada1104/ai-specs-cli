---
name: skills-as-rules
description: Interactively turn one project convention into a proper Agent Skill (uses skill-creator + skill-sync).
---

# /skills-as-rules

Goal: take ONE convention the dev wants the agent to respect (commits, an
architectural rule, testing approach, a deploy workflow, etc.) and formalize
it as a `SKILL.md` under `ai-specs/skills/`. The skill ends up reflected in
`AGENTS.md` (auto-invoke table) so every agent loads it on the right trigger.

This command is **interactive and Socratic**. Do NOT skip steps, do NOT batch
multiple skills in a single run, do NOT touch any file before the dev has
answered the questions below.

---

## Step 1 — Pick the convention (1 question)

Ask the dev, in their language:

> ¿Qué convención querés formalizar como skill?
> Ejemplos: reglas de commit, layering del backend, política de testing,
> workflow de releases, naming en el frontend, manejo de migraciones, ...

**Wait for the answer. One skill per run.** If the dev describes more than
one, pick the first and tell them to re-run `/skills-as-rules` for the rest.

---

## Step 2 — Extract the rules (1-3 questions, max)

Probe just enough to write the skill body well:

1. ¿Cuáles son las reglas concretas? (pedir ejemplos si la respuesta es vaga)
2. ¿Hay anti-patrones específicos que querés que el agente evite?
3. ¿Hay archivos / paths / herramientas particulares involucrados? (esto
   ayuda a escribir buenos `auto_invoke` triggers)

If the project already has docs that codify this (a `CONTRIBUTING.md`, a
`.cursor/rules/*.mdc`, an ADR), **read them** before asking — don't make the
dev repeat what's already written down. Just confirm and ask for gaps.

---

## Step 3 — Define when the skill auto-loads (1 question)

This is the most important question:

> ¿Cuándo querés que el agente cargue esta skill solo, sin que se la pidas?
> Ej: "cuando arme un commit", "al tocar archivos en apps/api/",
> "antes de abrir un PR", "cuando ejecute migraciones".

The answer becomes the `auto_invoke` list in the frontmatter. Each item
should be a short imperative trigger that maps cleanly to an action the
agent might be about to take.

---

## Step 4 — Author the skill

1. **Read the bundled author guide:**
   ```
   Read ai-specs/skills/skill-creator/SKILL.md
   ```

2. **Pick a name:** kebab-case, prefix with the project short name to avoid
   collisions with vendored skills. E.g. `<project>-commit`, `<project>-api`,
   `<project>-frontend-testing`.

3. **Create `ai-specs/skills/<name>/SKILL.md`** with the FULL frontmatter
   block (this is what makes it appear in AGENTS.md):

   ```yaml
   ---
   name: <name>
   description: >
     <one-paragraph: what this is + when to invoke it>.
     Trigger: <comma-separated list of situations that should auto-load this skill>.
   metadata:
     scope: [root]
     auto_invoke:
       - "<trigger 1 from Step 3>"
       - "<trigger 2 from Step 3>"
   allowed-tools: Read, Edit, Write, Glob, Grep, Bash
   ---
   ```

   Body: prescriptive and concrete. Lead with rules, follow with examples,
   close with anti-patterns. Drop tribal lore that has no operational impact.

---

## Step 5 — Sync AGENTS.md

1. **Read the sync guide:**
   ```
   Read ai-specs/skills/skill-sync/SKILL.md
   ```

2. Run its `assets/sync.sh`. The auto-invoke table in `AGENTS.md` should
   now include the new skill's triggers.

3. Verify by grepping `AGENTS.md` for the new skill name. If it's missing,
   the frontmatter is wrong (most likely missing `metadata.scope` or
   `metadata.auto_invoke`) — fix and re-run `sync.sh`.

---

## Step 6 — Hand off

Report back to the dev, briefly:

- ✓ Skill creada: `ai-specs/skills/<name>/SKILL.md`
- ✓ Auto-invoke registrado en `AGENTS.md`
- 📦 Para commitear: `ai-specs/skills/<name>/` y `AGENTS.md`
- 💡 Si querés formalizar otra convención, corré `/skills-as-rules` de nuevo

**Do NOT** delete or edit existing source files (`.cursorrules`,
`CONTRIBUTING.md`, etc.). Mention them as candidates for cleanup but let the
dev decide.

---

## Anti-patterns (for you, the agent)

- ❌ Crear varias skills en una sola corrida. Una skill por invocación.
- ❌ Saltarte el paso interactivo y armar la skill por inferencia.
- ❌ Frontmatter sin `metadata.scope` o `metadata.auto_invoke` — la skill no
  va a aparecer en `AGENTS.md` y el dev se va a confundir.
- ❌ Editar `AGENTS.md` a mano. Es regenerado. Siempre pasar por `skill-sync`.
- ❌ Tomar decisiones de scope/triggers sin confirmar con el dev.

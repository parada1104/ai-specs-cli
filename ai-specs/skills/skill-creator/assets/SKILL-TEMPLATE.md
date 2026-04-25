---
name: {skill-name}
description: >
 {Brief description of what this skill enables}.
 Trigger: {When the AI should load this skill - be specific}.
license: Apache-2.0
metadata:
 author: your-team
 version: "1.0"
 scope: [root]
 auto_invoke:
   - "{Primary action phrase for AGENTS.md sync}"
allowed-tools: Read, Edit, Write, Glob, Grep, Bash
---

## When to Use

Use this skill when:
- {Condition 1}
- {Condition 2}
- {Condition 3}

---

## Critical Patterns

{The MOST important rules - what AI MUST follow}

---

## Commands

```bash
{command 1}  # {description}
```

---

## Resources

- **Templates**: See [assets/](assets/) for optional files
- **Contract**: [../../contracts/skill-frontmatter.md](../../contracts/skill-frontmatter.md)

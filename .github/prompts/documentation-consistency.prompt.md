---
agent: agent
---

# Documentation Consistency Check

Systematically verify that all documentation in **CodeValdAI** is consistent,
current, and production-ready. Ask **one question at a time**.

---

## 🔄 MANDATORY REFACTOR WORKFLOW (ENFORCE BEFORE ANY CHECK)

```bash
wc -l documentation/3-SofwareDevelopment/mvp-details/{topic-file}.md
```

If >500 lines → refactor into topic-based subfolder before continuing.

---

## Question Categories (Execute in Order)

### 1. Technology Stack Consistency
Are there outdated references (e.g., wrong Go version, wrong gRPC version, old `AgentRunStatus` values, outdated proto signatures)?

### 2. Architecture Consistency
Does `documentation/2-SoftwareDesignAndArchitecture/` match:
- Current `AIManager` interface signature in `ai.go`?
- Current `LLMClient` and `CrossPublisher` interface contracts?
- Current `AgentRunStatus` constants in `models.go`?
- Current `DefaultAISchema` entity / edge definitions in `schema.go`?
- Current Cross topic naming (`cross.ai.{agencyID}.…`)?

### 3. Cross-Reference Validation
Do all links in README and index files point to existing documents?
Do `documentation/2-…` and `documentation/3-…` refer to the same file paths and proto messages?

### 4. File Organisation
Are there 3+ files on the same topic (provider catalogue, agent catalogue, run lifecycle) that should be in a subfolder?

### 5. File Size Compliance
Are any `.md` files exceeding 500 lines?

### 6. Naming Convention Compliance
All files should follow `kebab-case-descriptive-name.md`.

### 7. Content Duplication
Are any two files covering the same topic (e.g., run lifecycle described in two places that drift)?

### 8. Production Readiness
- [ ] gRPC service contract documented (proto file path + generated package path)
- [ ] Cross topics produced/consumed documented (topic, producer/consumer, payload shape)
- [ ] Run lifecycle state diagram present and matches `models.go`
- [ ] Graph topology diagram (LLMProvider ↔ Agent ↔ AgentRun ↔ RunField/RunInput) present and matches `schema.go`
- [ ] Error types documented (matches `errors.go`)
- [ ] Configuration options documented (matches `internal/config/config.go` and `.env.example`)
- [ ] LLM provider integration documented (Anthropic for MVP, provider-agnostic interface)
- [ ] Deployment / environment setup documented
- [ ] Testing strategy documented (unit + ArangoDB integration via `make test-arango`)

---

## Question Flow

**After each answer, explicitly choose:**

- 🔍 **DEEPER** — examine the specific file flagged
- 📝 **NOTE** — record inconsistency for action list
- ➡️ **NEXT** — no issues, move to next check
- 📊 **REVIEW** — summarise findings

---

## Issue Tracking

### 🚨 Inconsistencies Found
- 📝 **[File]**: Issue description

### ✅ Verified Clean
- ✅ **[Area]**: No issues found

### 🔄 Actions Required
- 🔧 Update: [files]
- 🔧 Organise: [folders]

**Update every 3-5 questions.**

---

## Completion Criteria

- ✅ All categories checked
- ✅ Architecture doc matches `ai.go`, `models.go`, `errors.go`, `schema.go`
- ✅ Run lifecycle and graph topology diagrams current
- ✅ Cross topic registry complete and current
- ✅ No broken links
- ✅ All files within size limits
- ✅ Production readiness checklist complete

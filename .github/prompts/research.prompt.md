---
agent: agent
---

# Research & Documentation Gap Analysis Prompt

## Purpose
This prompt guides a structured Q&A session to explore and complete documentation
for any feature or architectural component in **CodeValdAI** — the AI agent
management and execution service — through **one question at a time**, allowing
for deep dives into specific topics.

---

## 🔄 MANDATORY REFACTOR WORKFLOW (ENFORCE BEFORE ANY RESEARCH SESSION)

**BEFORE starting any research or writing new task documentation:**

### Step 1: CHECK File Size
```bash
wc -l documentation/3-SofwareDevelopment/mvp-details/{topic-file}.md
```

### Step 2: IF >500 lines OR individual MVP-XXX.md files exist:

**a. CREATE folder structure:**
```bash
documentation/3-SofwareDevelopment/mvp-details/{domain-name}/
├── README.md              # Domain overview, architecture, task index (MAX 300 lines)
├── {topic-1}.md           # Topic-based grouping of related tasks (MAX 500 lines)
├── {topic-2}.md           # Topic-based grouping of related tasks (MAX 500 lines)
├── architecture/          # Optional: detailed technical designs
│   ├── flow-diagrams.md
│   ├── data-models.md
│   └── state-machines.md
└── examples/              # Optional: config samples, proto snippets
    ├── sample-configs.yaml
    └── proto-examples.md
```

**b. CREATE README.md** with:
- Domain overview
- Architecture summary
- Task index with links

**c. SPLIT content by TOPIC (NOT by task ID):**
- Group related tasks into topic files
- Examples for CodeValdAI: `provider-catalogue.md`, `agent-catalogue.md`, `run-intake.md`, `run-execution.md`, `llm-client.md`, `cross-publisher.md`, `schema.md`

**d. MOVE architecture diagrams** → `architecture/` subfolder

**e. MOVE examples** → `examples/` subfolder

### Step 3: ONLY THEN add new task content to appropriate topic file

---

## 🛑 STOP CONDITIONS (Do NOT proceed until fixed)

- ❌ **Domain file exceeds 500 lines** → **MUST refactor first**
- ❌ **README.md exceeds 300 lines** → **MUST split content**
- ❌ **Individual `MVP-XXX.md` files exist** → **MUST consolidate by topic**
- ❌ **Task file exceeds 200 lines** → **MUST split into subtopics**

---

## Instructions for AI Assistant

Conduct a comprehensive documentation gap analysis through **iterative single-question
exploration**. Ask ONE question at a time, wait for the response, then decide whether to:

- **Go Deeper**: Ask follow-up questions on the same topic
- **Take Note**: Record a gap for later exploration
- **Move On**: Proceed to the next topic area
- **Review**: Summarize what we've learned and identify remaining gaps

---

## Research Framework

### 1. Session Initiation
1. **State the feature/component** being researched
2. **Scan existing documentation** and code quickly
3. **Ask the first question** from the most critical area
4. **Wait for response** before proceeding

### 2. Question Flow

**After each answer, explicitly choose one of these paths:**

- 🔍 **DEEPER**: "Let me dig deeper into [specific aspect]..."
- 📝 **NOTE**: "I'll note this gap: [description]..."
- ➡️ **NEXT**: "Moving to [new topic area]..."
- 📊 **REVIEW**: "Let me summarize what we've covered..."

### 3. CodeValdAI-Specific Question Categories (Priority Order)

#### Architecture & Design
- What is the lifecycle for this feature (e.g., from gRPC call → `AIManager` method → storage / LLM / Cross publish)?
- Which interface boundary does it sit behind (`AIManager`, `LLMClient`, `CrossPublisher`)?
- What graph entities and edges does it touch?
- What are the failure modes and retry strategies (especially for LLM calls)?

#### Run Lifecycle
- What `AgentRunStatus` transitions does this feature drive?
- What happens on partial failure (e.g., LLM call succeeds but storage write fails)?
- Is `ExecuteRun`'s `pending_intake` precondition still enforced?
- Are terminal states (`completed`, `failed`) treated as immutable?

#### LLM Client
- What request shape is sent to the LLM (system prompt + instructions + inputs)?
- What response shape is expected, and how is malformed output handled (`ErrInvalidLLMResponse`)?
- How are token counts captured and stored on the `AgentRun`?
- Is the `LLMClient` provider-agnostic — would swapping Anthropic for OpenAI require domain changes?

#### Cross Publisher
- Which Cross topic(s) does this feature publish to (`cross.ai.{agencyID}.{...}`)?
- What is the payload shape for each topic?
- Are publish failures logged and ignored (correct) or treated as fatal (incorrect)?
- Are topics consumed by AI (`cross.agency.created`, `work.task.dispatched`) wired up?

#### Storage / Schema
- What entity types and edges are involved (see `schema.go`)?
- Are linked references read through edges, not flat FK fields?
- Is the schema seeded idempotently in `cmd/main.go`?
- Are integration tests under `storage/arangodb/` and `internal/server/` covering this flow?

#### gRPC Contracts
- Which proto messages are involved (see `proto/codevaldai/v1/ai.proto`)?
- Is the generated code under `gen/go/` regenerated and committed?
- Does `internal/server/errors.go` map every relevant sentinel from `errors.go` to a gRPC status?

#### Data Models
- What value types (`models.go`) are needed?
- What error types (`errors.go`) should be defined?
- Are request/update structs minimal — only mutable fields on `Update*Request`?

#### Testing & Quality
- Are unit tests in place for the success path and each error path?
- Are integration tests via `make test-arango` covering ArangoDB writes?
- Is `LLMClient` mocked in unit tests (no live LLM calls)?
- Is `CrossPublisher` mocked in unit tests (no live gRPC to Cross)?

### 4. Single Question Format

```
🔍 [Category]

**Question**: [Your specific, singular question]

**Context**: [1-2 sentences on why this matters]

**What I'm Looking For**: [Expected type of answer]
```

### 5. Response Processing

After each answer:
1. **Acknowledge**: briefly confirm understanding
2. **Decide Path**: DEEPER / NOTE / NEXT / REVIEW
3. **State Choice Explicitly**
4. **Ask Next Question**

---

## Gap Tracking System

### Identified Gaps
- 📝 **[Topic]**: Brief description

### Explored Topics
- ✅ **[Topic]**: Sufficient understanding achieved

### Deep Dive Areas
- 🔍 **[Topic]**: Currently exploring

**Update this list every 3-5 questions.**

---

## Completion Criteria

- ✅ All critical question categories explored
- ✅ Run lifecycle and `AgentRunStatus` transitions understood
- ✅ LLM client and Cross publisher contracts clear
- ✅ Graph topology and schema entries documented
- ✅ Edge cases and error scenarios identified
- ✅ No blocking gaps remain
- ✅ User confirms readiness to conclude

---

## Session Control Commands

- 💬 `"review progress"` — show gap tracking summary
- 💬 `"switch to [topic]"` — change focus area
- 💬 `"go deeper"` — continue current topic
- 💬 `"skip this"` — note as gap and move on
- 💬 `"wrap up"` — conclude with summary

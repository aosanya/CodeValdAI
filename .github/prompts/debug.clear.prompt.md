---
agent: agent
---

# Clear Debug Artifacts

Remove temporary debug code added during a debug session in **CodeValdAI**.

## What to Remove

```bash
cd /workspaces/CodeVald-AIProject/CodeValdAI

# Find debug prints / temporary logs
grep -rn "fmt.Print\|log.Print\|fmt.Println\|log.Println\|DEBUG\|FIXME\|HACK\|TODO debug" \
  --include="*.go" \
  --exclude-dir=vendor \
  --exclude-dir=gen \
  .
```

## What to Keep

- ✅ Structured logger calls (`log.Printf`, `slog.Info`, `slog.Error`, `slog.Debug`) that log run IDs / agent IDs for tracing
- ✅ `// TODO:` comments tracking genuine future work
- ✅ Test helper functions in `_test.go` files
- ✅ Generated code under `gen/` — never hand-edit

## Pay Special Attention To

- ❌ Any `log.Printf` that prints an `LLMProvider.APIKey`, full request payloads, or full LLM responses
- ❌ Any `fmt.Println` left over from inspecting Cross publish payloads
- ❌ Any `t.Logf` left in non-test code (impossible — but check imports of `testing`)

## Validate After Cleanup

```bash
go build ./...           # must succeed
go vet ./...             # must show 0 issues
go test -v -race ./...   # must pass
golangci-lint run ./...  # must pass
```

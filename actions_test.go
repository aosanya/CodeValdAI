package codevaldai

import (
	"strings"
	"testing"
)

func TestParseActions_PostThinkValid(t *testing.T) {
	output := "<think>\nreasoning\n</think>\n\n```actions\n" +
		`[{"topic":"git.branch.create","payload":{"name":"feature/foo"}}]` +
		"\n```\n"
	actions, err := parseActions(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 1 || actions[0].Topic != "git.branch.create" {
		t.Fatalf("unexpected actions: %+v", actions)
	}
}

func TestParseActions_NoBlockReturnsNil(t *testing.T) {
	actions, err := parseActions("just prose, no fences here")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if actions != nil {
		t.Fatalf("expected nil actions, got %+v", actions)
	}
}

// BUG-09-028 — when the post-think actions block is truncated mid-emission
// (open fence, no close), parseActions should fall back to a complete
// actions block found inside <think>...</think>.
func TestParseActions_FallsBackToInThinkWhenPostThinkTruncated(t *testing.T) {
	output := "<think>\n" +
		"long reasoning ...\n" +
		"```actions\n" +
		`[{"topic":"todo.created","payload":{"title":"step 1"}},` +
		`{"topic":"todo.created","payload":{"title":"step 2"}}]` +
		"\n```\n" +
		"more reasoning ...\n" +
		"</think>\n\n" +
		"```actions\n" +
		`[{"topic":"todo.created","payload":{"title":"step 1"`
	actions, err := parseActions(output)
	if err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("expected 2 fallback actions, got %d: %+v", len(actions), actions)
	}
	if actions[0].Topic != "todo.created" || actions[1].Payload["title"] != "step 2" {
		t.Fatalf("unexpected fallback actions: %+v", actions)
	}
}

// When there is no in-think block to fall back to, the existing
// "open fence but no closing" error is preserved.
func TestParseActions_TruncatedPostThinkWithoutInThinkReturnsError(t *testing.T) {
	output := "<think>\nreasoning with no fenced block\n</think>\n\n" +
		"```actions\n" +
		`[{"topic":"todo.created"`
	_, err := parseActions(output)
	if err == nil {
		t.Fatal("expected error for unclosed actions block, got nil")
	}
	if !strings.Contains(err.Error(), "no closing") {
		t.Fatalf("expected 'no closing' error, got %v", err)
	}
}

// When post-think contains a valid block, the in-think block must NOT
// shadow it — post-think is canonical when complete.
func TestParseActions_PostThinkWinsWhenComplete(t *testing.T) {
	output := "<think>\n" +
		"```actions\n" +
		`[{"topic":"stale.in.think"}]` +
		"\n```\n</think>\n\n" +
		"```actions\n" +
		`[{"topic":"real.post.think"}]` +
		"\n```\n"
	actions, err := parseActions(output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 1 || actions[0].Topic != "real.post.think" {
		t.Fatalf("expected post-think to win, got %+v", actions)
	}
}

func TestParseActions_InvalidJSONReturnsError(t *testing.T) {
	output := "```actions\nnot json\n```"
	_, err := parseActions(output)
	if err == nil {
		t.Fatal("expected JSON decode error, got nil")
	}
}

func TestParseActions_EmptyBlockReturnsError(t *testing.T) {
	output := "```actions\n\n```"
	_, err := parseActions(output)
	if err == nil {
		t.Fatal("expected empty block error, got nil")
	}
}

func TestExtractThinkContents_HandlesUnclosedThink(t *testing.T) {
	got := extractThinkContents("prefix <think>inner content that never closes")
	if !strings.Contains(got, "inner content that never closes") {
		t.Fatalf("expected unclosed <think> contents, got %q", got)
	}
}

func TestExtractThinkContents_ConcatenatesMultipleBlocks(t *testing.T) {
	got := extractThinkContents("<think>one</think> mid <think>two</think>")
	if got != "onetwo" {
		t.Fatalf("expected 'onetwo', got %q", got)
	}
}

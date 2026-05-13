package codevaldai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// HydrateEventContext inspects the raw JSON event payload, fetches richer
// details for any known entity IDs, and returns a formatted context block
// ready for injection into the LLM system prompt.
//
// Currently enriches:
//   - TaskID      → fetches task title and description from CodeValdWork
//   - repositories → fetches all repos for the agency from CodeValdGit
//
// crossHTTPAddr is a base URL such as "http://localhost:8080".
// On any fetch error the function degrades gracefully — the raw payload
// is always included so the LLM still sees the event data.
func HydrateEventContext(ctx context.Context, crossHTTPAddr, agencyID, eventPayload string) string {
	var b strings.Builder
	b.WriteString("## Event Context\n")
	b.WriteString("Raw payload: ")
	b.WriteString(eventPayload)
	b.WriteString("\n")

	var raw map[string]string
	if err := json.Unmarshal([]byte(eventPayload), &raw); err != nil {
		return b.String()
	}

	if taskID, ok := raw["TaskID"]; ok && taskID != "" {
		if detail := fetchTaskDetail(ctx, crossHTTPAddr, agencyID, taskID); detail != "" {
			b.WriteString(detail)
		}
	}

	if repos := fetchRepositories(ctx, crossHTTPAddr, agencyID); repos != "" {
		b.WriteString(repos)
	}

	return b.String()
}

type repoListResponse struct {
	Repositories []struct {
		Name          string `json:"name"`
		DefaultBranch string `json:"defaultBranch"`
	} `json:"repositories"`
}

func fetchRepositories(ctx context.Context, crossHTTPAddr, agencyID string) string {
	url := fmt.Sprintf("%s/git/%s/repositories", crossHTTPAddr, agencyID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return ""
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	var r repoListResponse
	if err := json.Unmarshal(body, &r); err != nil || len(r.Repositories) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Repositories:\n")
	for _, repo := range r.Repositories {
		b.WriteString(fmt.Sprintf("  - name: %s  default_branch: %s\n", repo.Name, repo.DefaultBranch))
	}
	return b.String()
}

type taskDetail struct {
	Task struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Status      string `json:"status"`
		Priority    string `json:"priority"`
	} `json:"task"`
}

func fetchTaskDetail(ctx context.Context, crossHTTPAddr, agencyID, taskID string) string {
	url := fmt.Sprintf("%s/work/%s/tasks/%s", crossHTTPAddr, agencyID, taskID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return ""
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	var d taskDetail
	if err := json.Unmarshal(body, &d); err != nil {
		return ""
	}
	t := d.Task
	if t.ID == "" {
		return ""
	}
	return fmt.Sprintf("Task ID: %s\nTask Title: %s\nTask Description: %s\nTask Status: %s\n",
		t.ID, t.Title, t.Description, t.Status)
}

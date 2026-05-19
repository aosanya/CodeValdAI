package codevaldai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// HydrateEventContext inspects the raw JSON event payload, fetches richer
// details for any known entity IDs, and returns a formatted context block
// ready for injection into the LLM system prompt.
//
// Enriches:
//   - TodoID → fetches the TaskTodo to read Precalls specs and executes them
//   - TaskID / ParentTaskID → fetches task title, description, branch, project
//   - repositories → fetches all repos from CodeValdGit; uses project.repo_name
//     when available to pick the correct one
//   - branch files → if the task has a branch_name, fetches all text files on
//     that branch as a file dictionary so the LLM can work from in-memory
//     content and emit git.file.write actions to write back
//
// crossHTTPAddr is a base URL such as "http://localhost:8080".
// On any fetch error the function degrades gracefully.
func HydrateEventContext(ctx context.Context, crossHTTPAddr, agencyID, eventPayload string) string {
	var b strings.Builder
	b.WriteString("## Event Context\n")
	b.WriteString("Raw payload: ")
	b.WriteString(eventPayload)
	b.WriteString("\n")

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(eventPayload), &raw); err != nil {
		return b.String()
	}

	// ── Resolve TaskID from multiple possible field names ─────────────────────
	taskID := stringField(raw, "TaskID", "task_id", "ParentTaskID", "parent_task_id")
	todoID := stringField(raw, "TodoID", "todo_id")

	// ── Fetch task context ────────────────────────────────────────────────────
	var task *hydratedTask
	if taskID != "" {
		task = fetchTaskData(ctx, crossHTTPAddr, agencyID, taskID)
		if task != nil {
			b.WriteString(fmt.Sprintf("Task ID: %s\nTask Title: %s\nTask Description: %s\nTask Status: %s\n",
				task.ID, task.Title, task.Description, task.Status))
			if task.BranchName != "" {
				b.WriteString(fmt.Sprintf("Task Branch: %s\n", task.BranchName))
			}
			if task.ProjectName != "" {
				b.WriteString(fmt.Sprintf("Task Project: %s\n", task.ProjectName))
			}
		}
	}

	// ── Fetch repositories; pick the project repo when known ──────────────────
	repos := fetchRepoList(ctx, crossHTTPAddr, agencyID)
	var repoName string
	if len(repos) > 0 {
		b.WriteString("Repositories:\n")
		for _, r := range repos {
			b.WriteString(fmt.Sprintf("  - name: %s  default_branch: %s\n", r.Name, r.DefaultBranch))
		}
		// Prefer the repo linked to the project; fall back to the first repo.
		if task != nil && task.ProjectRepoName != "" {
			for _, r := range repos {
				if r.Name == task.ProjectRepoName {
					repoName = r.Name
					break
				}
			}
		}
		if repoName == "" {
			repoName = repos[0].Name
		}
	}

	// ── Execute precalls from the TaskTodo ───────────────────────────────────
	if todoID != "" && repoName != "" {
		precallCtx := fetchTodoPrecalls(ctx, crossHTTPAddr, agencyID, todoID)
		if precallCtx != "" {
			b.WriteString("\n")
			b.WriteString(precallCtx)
		}
	}

	// ── Pre-fetch branch file dictionary ─────────────────────────────────────
	if repoName != "" && task != nil && task.BranchName != "" {
		if dict := fetchBranchFileDict(ctx, crossHTTPAddr, agencyID, repoName, task.BranchName); dict != "" {
			b.WriteString("\n")
			b.WriteString(dict)
		}
	}

	return b.String()
}

// stringField extracts the first non-empty string value from raw for the given
// candidate field names. Returns "" when none match.
func stringField(raw map[string]json.RawMessage, names ...string) string {
	for _, name := range names {
		if v, ok := raw[name]; ok {
			var s string
			if err := json.Unmarshal(v, &s); err == nil && s != "" {
				return s
			}
		}
	}
	return ""
}

// hydratedTask holds the raw task data fetched from CodeValdWork.
type hydratedTask struct {
	ID              string
	Title           string
	Description     string
	Status          string
	BranchName      string
	ProjectName     string
	ProjectRepoName string // repo_name from the linked project
}

type taskDetailResponse struct {
	Task struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Status      string `json:"status"`
		BranchName  string `json:"branchName"`
		ProjectName string `json:"projectName"`
	} `json:"task"`
}

func fetchTaskData(ctx context.Context, crossHTTPAddr, agencyID, taskID string) *hydratedTask {
	taskURL := fmt.Sprintf("%s/work/%s/tasks/%s", crossHTTPAddr, agencyID, taskID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, taskURL, nil)
	if err != nil {
		return nil
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	var d taskDetailResponse
	if err := json.Unmarshal(body, &d); err != nil || d.Task.ID == "" {
		return nil
	}
	ht := &hydratedTask{
		ID:          d.Task.ID,
		Title:       d.Task.Title,
		Description: d.Task.Description,
		Status:      d.Task.Status,
		BranchName:  d.Task.BranchName,
		ProjectName: d.Task.ProjectName,
	}
	// If the task belongs to a project, fetch the project to get repo_name.
	if d.Task.ProjectName != "" {
		ht.ProjectRepoName = fetchProjectRepoName(ctx, crossHTTPAddr, agencyID, d.Task.ProjectName)
	}
	return ht
}

type projectDetailResponse struct {
	Project struct {
		RepoName string `json:"repoName"`
	} `json:"project"`
}

// fetchProjectRepoName resolves the CodeValdGit repository name linked to a
// project by its URL-safe project_name slug.
func fetchProjectRepoName(ctx context.Context, crossHTTPAddr, agencyID, projectName string) string {
	projURL := fmt.Sprintf("%s/work/%s/projects?project_name=%s", crossHTTPAddr, agencyID, url.QueryEscape(projectName))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, projURL, nil)
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
	// Try single-project response first.
	var single projectDetailResponse
	if err := json.Unmarshal(body, &single); err == nil && single.Project.RepoName != "" {
		return single.Project.RepoName
	}
	// Try list response.
	var list struct {
		Projects []struct {
			RepoName string `json:"repoName"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(body, &list); err == nil && len(list.Projects) > 0 {
		return list.Projects[0].RepoName
	}
	return ""
}

type repoInfo struct {
	Name          string `json:"name"`
	DefaultBranch string `json:"defaultBranch"`
}

type repoListResponse struct {
	Repositories []repoInfo `json:"repositories"`
}

func fetchRepoList(ctx context.Context, crossHTTPAddr, agencyID string) []repoInfo {
	repoURL := fmt.Sprintf("%s/git/%s/repositories", crossHTTPAddr, agencyID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, repoURL, nil)
	if err != nil {
		return nil
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	var r repoListResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil
	}
	return r.Repositories
}

// ── Precall execution ──────────────────────────────────────────────────────────

// PrecallSpec is a pre-execution fetch spec stored on a TaskTodo's precalls
// field. HydrateEventContext reads these and injects the results into the LLM
// context before the agent runs.
type PrecallSpec struct {
	// Service is the routing key for Cross: "git", "work", etc.
	Service string `json:"service"`
	// Operation is the specific fetch to execute, e.g. "blob_search".
	Operation string `json:"operation"`
	// Query is the search term (used by blob_search).
	Query string `json:"query,omitempty"`
	// Label is a human-readable title for the injected result block.
	Label string `json:"label,omitempty"`
}

type todoDetailResponse struct {
	Todo struct {
		Precalls string `json:"precalls"`
	} `json:"todo"`
}

// fetchTodoPrecalls reads the TaskTodo entity, parses its precalls field, and
// executes each spec against Cross HTTP. Returns a formatted Markdown block
// with all precall results, or "" when there are no precalls or all fail.
func fetchTodoPrecalls(ctx context.Context, crossHTTPAddr, agencyID, todoID string) string {
	todoURL := fmt.Sprintf("%s/work/%s/todos/%s", crossHTTPAddr, agencyID, todoID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, todoURL, nil)
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
	var d todoDetailResponse
	if err := json.Unmarshal(body, &d); err != nil || d.Todo.Precalls == "" {
		return ""
	}
	var specs []PrecallSpec
	if err := json.Unmarshal([]byte(d.Todo.Precalls), &specs); err != nil || len(specs) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Precall Results\n")
	wrote := false
	for _, spec := range specs {
		result := executePrecall(ctx, crossHTTPAddr, agencyID, spec)
		if result == "" {
			continue
		}
		label := spec.Label
		if label == "" {
			label = fmt.Sprintf("%s/%s", spec.Service, spec.Operation)
		}
		b.WriteString(fmt.Sprintf("### %s\n", label))
		b.WriteString(result)
		b.WriteString("\n")
		wrote = true
	}
	if !wrote {
		return ""
	}
	return b.String()
}

// blobSearchResponse mirrors the Cross SearchBlobsResponse JSON shape.
type blobSearchResponse struct {
	Results []struct {
		ID        string  `json:"id"`
		Path      string  `json:"path"`
		Name      string  `json:"name"`
		Extension string  `json:"extension"`
		Snippet   string  `json:"snippet"`
		Score     float64 `json:"score"`
	} `json:"results"`
}

// executePrecall dispatches a single PrecallSpec against Cross HTTP and returns
// a formatted Markdown string with the result, or "" on error/no results.
func executePrecall(ctx context.Context, crossHTTPAddr, agencyID string, spec PrecallSpec) string {
	switch spec.Service {
	case "git":
		return executeGitPrecall(ctx, crossHTTPAddr, agencyID, spec)
	default:
		return ""
	}
}

func executeGitPrecall(ctx context.Context, crossHTTPAddr, agencyID string, spec PrecallSpec) string {
	switch spec.Operation {
	case "blob_search":
		return executeBlobSearch(ctx, crossHTTPAddr, agencyID, spec.Query)
	default:
		return ""
	}
}

func executeBlobSearch(ctx context.Context, crossHTTPAddr, agencyID, query string) string {
	if query == "" {
		return ""
	}
	// Use "_" as the repo placeholder for agency-wide search. SearchBlobs
	// accepts but ignores RepositoryName — the ArangoSearch View is agency-scoped.
	searchURL := fmt.Sprintf("%s/git/%s/repositories/_/blobs/search?query=%s&limit=20",
		crossHTTPAddr, agencyID, url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
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
	var r blobSearchResponse
	if err := json.Unmarshal(body, &r); err != nil || len(r.Results) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Search query: `%s` — %d result(s)\n\n", query, len(r.Results)))
	for _, res := range r.Results {
		b.WriteString(fmt.Sprintf("- **`%s`** (score: %.2f)\n", res.Path, res.Score))
		if res.Snippet != "" {
			b.WriteString(fmt.Sprintf("  > %s\n", strings.ReplaceAll(res.Snippet, "\n", " ")))
		}
	}
	return b.String()
}

// ── Branch file dictionary ─────────────────────────────────────────────────────

// listDirEntry is a single item returned by the Cross ListDirectory endpoint.
type listDirEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
}

type listDirResponse struct {
	Entries []listDirEntry `json:"entries"`
}

// blobFileResponse is the Cross ReadFile endpoint response.
type blobFileResponse struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

// fetchBranchFileDict fetches all text files on branchName (up to 50 files,
// 10 KB each) and returns a Markdown block with the full file dictionary,
// plus instructions for writing back via git.file.write actions.
// Binary files (encoding=="base64") are skipped. Returns "" when the branch
// has no files or any fetch fails.
func fetchBranchFileDict(ctx context.Context, crossHTTPAddr, agencyID, repoName, branchName string) string {
	const maxFiles = 50
	const maxFileBytes = 10 * 1024

	filePaths := listAllFiles(ctx, crossHTTPAddr, agencyID, repoName, branchName, "", maxFiles)
	if len(filePaths) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("## Working Branch: `%s`\n", branchName))
	b.WriteString("The following files are loaded from ArangoDB (your in-memory workspace). ")
	b.WriteString("Read them to understand the current state. Use `git.file.write` actions to write changes back — each action creates a real git commit in the repository.\n\n")

	written := 0
	for _, path := range filePaths {
		blob, ok := readBlobContent(ctx, crossHTTPAddr, agencyID, repoName, branchName, path)
		if !ok || blob.Encoding == "base64" {
			continue
		}
		content := blob.Content
		truncated := false
		if len(content) > maxFileBytes {
			content = content[:maxFileBytes]
			truncated = true
		}
		b.WriteString(fmt.Sprintf("### `%s`\n```\n%s\n```", path, content))
		if truncated {
			b.WriteString("\n_[truncated at 10 KB]_")
		}
		b.WriteString("\n\n")
		written++
	}

	if written == 0 {
		return ""
	}

	b.WriteString("---\n")
	b.WriteString("**Writing files back (creates a git commit):**\n")
	b.WriteString("```json\n")
	b.WriteString(fmt.Sprintf(
		`{"topic":"git.file.write","payload":{"repository":%q,"branch_name":%q,"path":"<relative-path>","content":"<full-file-content>","message":"<commit message>"}}`,
		repoName, branchName,
	))
	b.WriteString("\n```\n")
	b.WriteString("Emit one `git.file.write` action per file changed. Each write creates a commit on the branch.\n")

	return b.String()
}

// listAllFiles returns the full list of file paths on the branch by recursively
// listing directories via Cross HTTP. Stops when maxFiles is reached.
func listAllFiles(ctx context.Context, crossHTTPAddr, agencyID, repoName, branchName, dir string, maxFiles int) []string {
	treeURL := fmt.Sprintf("%s/git/%s/repositories/%s/branches/%s/tree",
		crossHTTPAddr, agencyID, repoName, url.PathEscape(branchName))
	if dir != "" {
		treeURL += "?path=" + url.QueryEscape(dir)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, treeURL, nil)
	if err != nil {
		return nil
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	var r listDirResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil
	}

	var paths []string
	for _, entry := range r.Entries {
		if len(paths) >= maxFiles {
			break
		}
		if entry.IsDir {
			sub := listAllFiles(ctx, crossHTTPAddr, agencyID, repoName, branchName, entry.Path, maxFiles-len(paths))
			paths = append(paths, sub...)
		} else {
			paths = append(paths, entry.Path)
		}
	}
	return paths
}

// readBlobContent fetches a single file's content from the branch via Cross HTTP.
func readBlobContent(ctx context.Context, crossHTTPAddr, agencyID, repoName, branchName, path string) (blobFileResponse, bool) {
	fileURL := fmt.Sprintf("%s/git/%s/repositories/%s/branches/%s/files?path=%s",
		crossHTTPAddr, agencyID, repoName, url.PathEscape(branchName), url.QueryEscape(path))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return blobFileResponse{}, false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return blobFileResponse{}, false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return blobFileResponse{}, false
	}
	var blob blobFileResponse
	if err := json.Unmarshal(body, &blob); err != nil {
		return blobFileResponse{}, false
	}
	return blob, true
}

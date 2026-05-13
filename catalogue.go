package codevaldai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// registryEntry mirrors the JSON shape returned by
// GET /services/registry?agencyId={id} on CodeValdCross.
type registryEntry struct {
	ServiceName  string            `json:"ServiceName"`
	Consumes     []string          `json:"Consumes"`
	Produces     []string          `json:"Produces"`
	TopicSchemas map[string]string `json:"topic_schemas,omitempty"`
}

// FetchActionCatalogue calls the CodeValdCross HTTP registry endpoint and
// returns CatalogueEntry records for every topic each service consumes or
// produces. crossHTTPAddr must be a base URL such as "http://localhost:8080".
// On error the function returns an empty slice so the caller degrades
// gracefully — the LLM still runs, just without the action catalogue.
func FetchActionCatalogue(ctx context.Context, crossHTTPAddr, agencyID string) []CatalogueEntry {
	url := fmt.Sprintf("%s/services/registry?agencyId=%s", crossHTTPAddr, agencyID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	var entries []registryEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil
	}

	var out []CatalogueEntry
	for _, svc := range entries {
		for _, t := range svc.Consumes {
			out = append(out, CatalogueEntry{
				ServiceName: svc.ServiceName,
				Topic:       t,
				Direction:   "consumes",
				Schema:      svc.TopicSchemas[t],
			})
		}
		for _, t := range svc.Produces {
			out = append(out, CatalogueEntry{ServiceName: svc.ServiceName, Topic: t, Direction: "produces"})
		}
	}
	return out
}

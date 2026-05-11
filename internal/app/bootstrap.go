// Package app — bootstrap.go
// Reads an agency.json at startup and idempotently provisions the LLM
// providers and agents declared in its ai_config section. After provisioning,
// wires each agent's runtime ID back to its matching work plan in
// CodeValdAgency so the dispatcher can route events correctly.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	codevaldai "github.com/aosanya/CodeValdAI"
	agencypb "github.com/aosanya/CodeValdAgency/gen/go/codevaldagency/v1"
)

// ── agency.json minimal schema ────────────────────────────────────────────────

type agencyJSONAIConfig struct {
	AIConfig  aiConfigSpec    `json:"ai_config"`
	WorkPlans []wpMinimalSpec `json:"work_plans"`
}

type aiConfigSpec struct {
	Providers []aiProviderSpec `json:"providers"`
	Agents    []aiAgentSpec    `json:"agents"`
}

type aiProviderSpec struct {
	Code          string `json:"code"`
	Name          string `json:"name"`
	ProviderType  string `json:"provider_type"`
	APIKeyEnv     string `json:"api_key_env"`
	BaseURL       string `json:"base_url"`
	ProviderRoute string `json:"provider_route"`
}

type aiAgentSpec struct {
	Code         string  `json:"code"`
	Name         string  `json:"name"`
	ProviderCode string  `json:"provider_code"`
	Model        string  `json:"model"`
	SystemPrompt string  `json:"system_prompt"`
	Temperature  float64 `json:"temperature"`
	MaxTokens    int     `json:"max_tokens"`
}

type wpMinimalSpec struct {
	Name      string `json:"name"`
	AgentCode string `json:"agent_code"`
}

// ── Bootstrap ─────────────────────────────────────────────────────────────────

// bootstrapAIConfig reads the agency.json at path, provisions providers and
// agents from ai_config (idempotent by name), then wires each agent ID to its
// matching work plan via agencyClient. agencyClient may be nil — wiring is
// skipped in that case. Errors are logged but do not block startup.
func bootstrapAIConfig(ctx context.Context, path string, mgr codevaldai.AIManager, agencyClient agencypb.AgencyServiceClient) {
	if err := doBootstrap(ctx, path, mgr, agencyClient); err != nil {
		log.Printf("codevaldai: ai_config bootstrap: %v", err)
	}
}

func doBootstrap(ctx context.Context, path string, mgr codevaldai.AIManager, agencyClient agencypb.AgencyServiceClient) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	var spec agencyJSONAIConfig
	if err := json.Unmarshal(data, &spec); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	if len(spec.AIConfig.Providers) == 0 && len(spec.AIConfig.Agents) == 0 {
		log.Println("codevaldai: ai_config: no providers or agents — skipping bootstrap")
		return nil
	}

	// ── Providers (idempotent by name) ────────────────────────────────────────
	existingProviders, err := mgr.ListProviders(ctx)
	if err != nil {
		return fmt.Errorf("list providers: %w", err)
	}
	providerNameToID := make(map[string]string, len(existingProviders))
	for _, p := range existingProviders {
		providerNameToID[p.Name] = p.ID
	}

	providerCodeToID := make(map[string]string, len(spec.AIConfig.Providers))
	for _, ps := range spec.AIConfig.Providers {
		if id, ok := providerNameToID[ps.Name]; ok {
			providerCodeToID[ps.Code] = id
			log.Printf("codevaldai: ai_config: provider %q already exists (id=%s)", ps.Name, id)
			continue
		}
		p, err := mgr.CreateProvider(ctx, codevaldai.CreateProviderRequest{
			Name:          ps.Name,
			ProviderType:  ps.ProviderType,
			APIKey:        os.Getenv(ps.APIKeyEnv),
			BaseURL:       ps.BaseURL,
			ProviderRoute: ps.ProviderRoute,
		})
		if err != nil {
			return fmt.Errorf("create provider %q: %w", ps.Name, err)
		}
		providerCodeToID[ps.Code] = p.ID
		log.Printf("codevaldai: ai_config: created provider %q (id=%s)", ps.Name, p.ID)
	}

	// ── Agents (idempotent by name) ───────────────────────────────────────────
	existingAgents, err := mgr.ListAgents(ctx)
	if err != nil {
		return fmt.Errorf("list agents: %w", err)
	}
	agentNameToID := make(map[string]string, len(existingAgents))
	for _, a := range existingAgents {
		agentNameToID[a.Name] = a.ID
	}

	agentCodeToID := make(map[string]string, len(spec.AIConfig.Agents))
	for _, as := range spec.AIConfig.Agents {
		if id, ok := agentNameToID[as.Name]; ok {
			agentCodeToID[as.Code] = id
			log.Printf("codevaldai: ai_config: agent %q already exists (id=%s)", as.Name, id)
			continue
		}
		providerID, ok := providerCodeToID[as.ProviderCode]
		if !ok {
			return fmt.Errorf("agent %q: unknown provider_code %q", as.Name, as.ProviderCode)
		}
		a, err := mgr.CreateAgent(ctx, codevaldai.CreateAgentRequest{
			Name:         as.Name,
			ProviderID:   providerID,
			Model:        as.Model,
			SystemPrompt: as.SystemPrompt,
			Temperature:  as.Temperature,
			MaxTokens:    as.MaxTokens,
		})
		if err != nil {
			return fmt.Errorf("create agent %q: %w", as.Name, err)
		}
		agentCodeToID[as.Code] = a.ID
		log.Printf("codevaldai: ai_config: created agent %q (id=%s)", as.Name, a.ID)
	}

	// ── Wire agent IDs to work plans ──────────────────────────────────────────
	if agencyClient == nil {
		return nil
	}

	wpNameToAgentCode := make(map[string]string)
	for _, wp := range spec.WorkPlans {
		if wp.AgentCode != "" {
			wpNameToAgentCode[wp.Name] = wp.AgentCode
		}
	}
	if len(wpNameToAgentCode) == 0 {
		return nil
	}

	resp, err := agencyClient.ListWorkPlans(ctx, &agencypb.ListWorkPlansRequest{})
	if err != nil {
		return fmt.Errorf("list work plans: %w", err)
	}

	for _, wp := range resp.GetWorkPlans() {
		agentCode, ok := wpNameToAgentCode[wp.GetName()]
		if !ok {
			continue
		}
		if wp.GetAgentId() != "" {
			log.Printf("codevaldai: ai_config: work plan %q already wired (agent_id=%s) — skipping", wp.GetName(), wp.GetAgentId())
			continue
		}
		agentID, ok := agentCodeToID[agentCode]
		if !ok {
			log.Printf("codevaldai: ai_config: work plan %q: agent_code %q not provisioned — skipping", wp.GetName(), agentCode)
			continue
		}
		if _, err := agencyClient.UpdateWorkPlan(ctx, &agencypb.UpdateWorkPlanRequest{
			WorkPlanId: wp.GetId(),
			AgentId:    agentID,
		}); err != nil {
			log.Printf("codevaldai: ai_config: wire work plan %q → agent %q: %v", wp.GetName(), agentID, err)
			continue
		}
		log.Printf("codevaldai: ai_config: wired work plan %q → agent %q (id=%s)", wp.GetName(), agentCode, agentID)
	}

	return nil
}

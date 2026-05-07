package server

import (
	"context"
	"fmt"
	"log"
	"strings"

	codevaldai "github.com/aosanya/CodeValdAI"
	agencypb "github.com/aosanya/CodeValdAgency/gen/go/codevaldagency/v1"
	"google.golang.org/grpc"
)

// EventDispatcher is called asynchronously after a ReceivedEvent is written.
// The dispatcher fans out IntakeRun + ExecuteRunStreaming per matched WorkPlan.
// A nil dispatcher disables dispatch (store-only mode).
type EventDispatcher interface {
	Dispatch(ctx context.Context, topic, payload string)
}

// workPlansMatcher is the subset of agencypb.AgencyServiceClient used for
// dispatch. Using a narrow interface keeps tests simple.
type workPlansMatcher interface {
	MatchWorkPlans(ctx context.Context, in *agencypb.MatchWorkPlansRequest, opts ...grpc.CallOption) (*agencypb.MatchWorkPlansResponse, error)
}

// RACIDispatcher implements EventDispatcher via Agency.MatchWorkPlans.
// For each matched WorkPlan that has an agent_id, it triggers a full two-phase
// run (IntakeRun → ExecuteRunStreaming) in a separate goroutine.
type RACIDispatcher struct {
	agencyClient workPlansMatcher
	mgr          codevaldai.AIManager
	agencyID     string
}

// NewRACIDispatcher returns an EventDispatcher backed by an Agency gRPC client.
// agencyClient is typically agencypb.NewAgencyServiceClient(conn).
func NewRACIDispatcher(agencyClient workPlansMatcher, mgr codevaldai.AIManager, agencyID string) *RACIDispatcher {
	return &RACIDispatcher{agencyClient: agencyClient, mgr: mgr, agencyID: agencyID}
}

// Dispatch calls Agency.MatchWorkPlans for the incoming topic+payload and fires
// one goroutine per matched WorkPlan. It returns immediately so NotifyEvent is
// not blocked.
func (d *RACIDispatcher) Dispatch(ctx context.Context, topic, payload string) {
	resp, err := d.agencyClient.MatchWorkPlans(ctx, &agencypb.MatchWorkPlansRequest{
		Topic:   topic,
		Payload: payload,
	})
	if err != nil {
		log.Printf("codevaldai: dispatch: MatchWorkPlans topic=%q: %v", topic, err)
		return
	}
	for _, match := range resp.GetMatches() {
		match := match
		go func() {
			if err := d.triggerPlanRun(context.Background(), match, topic, payload); err != nil {
				log.Printf("codevaldai: dispatch: plan=%s agent=%s: %v",
					match.GetWorkPlan().GetId(), match.GetWorkPlan().GetAgentId(), err)
			}
		}()
	}
}

// triggerPlanRun runs the two-phase pipeline for a single matched work plan.
func (d *RACIDispatcher) triggerPlanRun(ctx context.Context, match *agencypb.WorkPlanMatch, topic, payload string) error {
	plan := match.GetWorkPlan()
	if plan.GetAgentId() == "" {
		return nil // plan has no agent configured; nothing to dispatch
	}

	instructions := buildDispatchInstructions(plan, match.GetContextSources(), topic, payload)

	run, _, err := d.mgr.IntakeRun(ctx, codevaldai.IntakeRunRequest{
		AgentID:      plan.GetAgentId(),
		Instructions: instructions,
	})
	if err != nil {
		return fmt.Errorf("IntakeRun: %w", err)
	}

	if _, err := d.mgr.ExecuteRunStreaming(ctx, run.ID, nil, func(string) {}); err != nil {
		return fmt.Errorf("ExecuteRunStreaming run=%s: %w", run.ID, err)
	}
	return nil
}

// buildDispatchInstructions assembles the prompt string forwarded to IntakeRun:
// plan instructions + event topic + raw JSON payload + context source descriptions.
func buildDispatchInstructions(plan *agencypb.WorkPlan, sources []*agencypb.ContextSource, topic, payload string) string {
	var b strings.Builder
	if instr := plan.GetInstructions(); instr != "" {
		b.WriteString(instr)
		b.WriteString("\n\n")
	}
	b.WriteString("Event topic: ")
	b.WriteString(topic)
	b.WriteString("\nEvent payload:\n")
	b.WriteString(payload)
	if len(sources) > 0 {
		b.WriteString("\n\nContext sources:")
		for _, src := range sources {
			b.WriteString("\n- ")
			b.WriteString(src.GetSourceType())
			switch src.GetSourceType() {
			case "GitContextSource":
				if g := src.GetGit(); g != nil && g.GetSignals() != "" {
					b.WriteString(" (signals: " + g.GetSignals() + ")")
				}
			case "CommContextSource":
				if c := src.GetComm(); c != nil && c.GetLookbackDays() > 0 {
					b.WriteString(fmt.Sprintf(" (lookback: %dd)", c.GetLookbackDays()))
				}
			case "WorkContextSource":
				if w := src.GetWork(); w != nil {
					if w.GetIncludeDescription() {
						b.WriteString(" (include_description)")
					}
					if w.GetIncludeHistory() {
						b.WriteString(" (include_history)")
					}
				}
			}
		}
	}
	return b.String()
}

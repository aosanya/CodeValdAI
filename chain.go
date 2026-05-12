package codevaldai

import (
	"context"
	"sort"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// loadChainHistory returns all AgentRun entities in the chain with chain_id
// matching chainID and segment_number < maxSegment, sorted ascending by
// segment_number. Only YIELDED runs carry a partial_output worth replaying.
func (m *aiManager) loadChainHistory(ctx context.Context, chainID string, maxSegment int) ([]AgentRun, error) {
	entities, err := m.dm.ListEntities(ctx, entitygraph.EntityFilter{
		AgencyID: m.agencyID,
		TypeID:   "AgentRun",
	})
	if err != nil {
		return nil, err
	}

	var runs []AgentRun
	for _, e := range entities {
		r := agentRunFromEntity(e)
		if r.ChainID != chainID || r.SegmentNumber >= maxSegment {
			continue
		}
		runs = append(runs, r)
	}
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].SegmentNumber < runs[j].SegmentNumber
	})
	return runs, nil
}

// buildChainConversation constructs the []ConversationTurn slice passed to
// callLLM for session N+1. Each prior yielded segment contributes one
// user turn (the original instructions) and one assistant turn (partial output).
func buildChainConversation(priorRuns []AgentRun, currentInstructions string) []ConversationTurn {
	turns := make([]ConversationTurn, 0, len(priorRuns)*2)
	for _, r := range priorRuns {
		turns = append(turns,
			ConversationTurn{Role: "user", Content: r.Instructions},
			ConversationTurn{Role: "assistant", Content: r.PartialOutput},
		)
	}
	// The final user turn (current session's instructions) is passed separately
	// as the `user` param to callLLM — it is NOT appended here.
	_ = currentInstructions
	return turns
}

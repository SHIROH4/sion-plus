package cognition

import (
	"math"

	"github.com/SHIROH4/sion-plus/internal/domain/types"
)

// ConstrainedBanditConfig defines promotion and exploration limits for the
// contextual action-ranking candidate. It cannot bypass delivery hard gates.
type ConstrainedBanditConfig struct {
	MinTotalSamples   int
	MinActionSamples  int
	MinCoveredActions int
	ExplorationScale  float64
	MaxScoreDelta     float64
}

func DefaultConstrainedBanditConfig() ConstrainedBanditConfig {
	return ConstrainedBanditConfig{
		MinTotalSamples: 50, MinActionSamples: 5, MinCoveredActions: 3,
		ExplorationScale: 0.04, MaxScoreDelta: 0.10,
	}
}

// BanditShadowResult is observational. Ready only means the minimum data gate
// has passed; promotion still requires offline review and an explicit rollout.
type BanditShadowResult struct {
	Actions        []types.ScoredAction
	Ready          bool
	TotalSamples   int
	CoveredActions int
}

// RankConstrainedBanditShadow applies a conservative Bayesian-UCB-style score
// adjustment to communicative actions. Local context statistics take priority
// after MinActionSamples, otherwise global statistics are used. Silence is not
// assigned a synthetic reward, so the candidate only learns which content type
// to choose; whether to interrupt remains controlled by rules and hard gates.
func RankConstrainedBanditShadow(scored []types.ScoredAction, local, global []types.ActionFeedbackStats, cfg ConstrainedBanditConfig) BanditShadowResult {
	localByAction := statsByAction(local)
	globalByAction := statsByAction(global)
	total := 0
	covered := 0
	for _, stat := range global {
		total += stat.Samples
		if stat.Samples >= cfg.MinActionSamples {
			covered++
		}
	}

	for i := range scored {
		if scored[i].Action.OutcomeType == "silent" || scored[i].Action.Name == "none" {
			continue
		}
		stat, scope := localByAction[scored[i].Action.Name], "context"
		if stat.Samples < cfg.MinActionSamples {
			stat, scope = globalByAction[scored[i].Action.Name], "global"
		}
		// The prior is neutral and the exploration term is capped. Unobserved
		// actions can surface in shadow mode without destabilizing production.
		meanDelta := preferenceBias(stat) * (cfg.MaxScoreDelta / 0.12)
		uncertainty := math.Sqrt(math.Log(float64(total)+2) / float64(stat.Samples+2))
		delta := meanDelta + cfg.ExplorationScale*uncertainty
		if delta > cfg.MaxScoreDelta {
			delta = cfg.MaxScoreDelta
		}
		if delta < -cfg.MaxScoreDelta {
			delta = -cfg.MaxScoreDelta
		}
		scored[i].FinalScore += delta
		scored[i].Modulators["bandit_shadow_"+scope] = 1 + delta
	}
	sortScoredActions(scored)
	return BanditShadowResult{
		Actions: scored, TotalSamples: total, CoveredActions: covered,
		Ready: total >= cfg.MinTotalSamples && covered >= cfg.MinCoveredActions,
	}
}

func statsByAction(stats []types.ActionFeedbackStats) map[string]types.ActionFeedbackStats {
	result := make(map[string]types.ActionFeedbackStats, len(stats))
	for _, stat := range stats {
		result[stat.Action] = stat
	}
	return result
}

package cognition

import "github.com/SHIROH4/sion-plus/internal/domain/types"

// BuildActions returns the complete 16-action registry.
// This is the CANONICAL action set. Adding/removing/changing actions = editing this function.
func BuildActions() []types.ActionDef {
	return []types.ActionDef{
		{Name: "speak_casual", Category: "social", NightSafe: false, OutcomeType: "speak", Source: "proactive",
			WeightSocial: 0.80, WeightCare: 0.15, WeightCurious: 0.05, WeightQuiet: -0.30, WeightExplore: 0.00,
			SkillCard: types.SkillCard{Trigger: "User is relaxed and open to chat.", Action: "Start a casual observation or light question.", Delivery: "Short and warm, playful tone."}},
		{Name: "speak_care", Category: "social", NightSafe: false, OutcomeType: "speak", Source: "proactive",
			WeightSocial: 0.40, WeightCare: 0.70, WeightCurious: 0.00, WeightQuiet: -0.20, WeightExplore: 0.00,
			SkillCard: types.SkillCard{Trigger: "User shows signs of stress or long work hours.", Action: "Express concern, suggest a break.", Delivery: "Gentle and warm, not pushy."}},
		{Name: "speak_inquiry", Category: "social", NightSafe: false, OutcomeType: "speak", Source: "proactive",
			WeightSocial: 0.40, WeightCare: 0.00, WeightCurious: 0.60, WeightQuiet: 0.00, WeightExplore: 0.10,
			SkillCard: types.SkillCard{Trigger: "AI has an active curiosity goal.", Action: "Ask an open-ended question.", Delivery: "Curious but not interrogating."}},
		{Name: "care_rest", Category: "care", NightSafe: true, OutcomeType: "speak", Source: "proactive",
			WeightSocial: 0.10, WeightCare: 0.75, WeightCurious: 0.00, WeightQuiet: 0.00, WeightExplore: 0.00,
			SkillCard: types.SkillCard{Trigger: "User working >2h continuously.", Action: "Gently suggest a break.", Delivery: "Soft and caring."}},
		{Name: "care_meal", Category: "care", NightSafe: false, OutcomeType: "speak", Source: "proactive",
			WeightSocial: 0.10, WeightCare: 0.70, WeightCurious: 0.00, WeightQuiet: 0.00, WeightExplore: 0.00,
			SkillCard: types.SkillCard{Trigger: "Meal time and user seems working.", Action: "Remind to eat.", Delivery: "Cheerful and caring."}},
		{Name: "care_hydration", Category: "care", NightSafe: true, OutcomeType: "speak", Source: "proactive",
			WeightSocial: 0.05, WeightCare: 0.65, WeightCurious: 0.00, WeightQuiet: 0.00, WeightExplore: 0.00,
			SkillCard: types.SkillCard{Trigger: "Periodic hydration reminder.", Action: "Suggest drinking water.", Delivery: "Quick and gentle."}},
		{Name: "care_health", Category: "care", NightSafe: true, OutcomeType: "speak", Source: "proactive",
			WeightSocial: 0.05, WeightCare: 0.65, WeightCurious: 0.00, WeightQuiet: 0.00, WeightExplore: 0.00,
			SkillCard: types.SkillCard{Trigger: "User mentioned health/fatigue recently.", Action: "Check in on health.", Delivery: "Warm and concerned."}},
		{Name: "care_encourage", Category: "care", NightSafe: false, OutcomeType: "speak", Source: "proactive",
			WeightSocial: 0.20, WeightCare: 0.55, WeightCurious: 0.00, WeightQuiet: 0.00, WeightExplore: 0.00,
			SkillCard: types.SkillCard{Trigger: "User seems discouraged or frustrated.", Action: "Offer encouragement.", Delivery: "Empathetic, acknowledge difficulty first."}},
		{Name: "care_social", Category: "care", NightSafe: false, OutcomeType: "speak", Source: "proactive",
			WeightSocial: 0.30, WeightCare: 0.40, WeightCurious: 0.00, WeightQuiet: 0.00, WeightExplore: 0.00,
			SkillCard: types.SkillCard{Trigger: "User seems lonely or working alone long.", Action: "Provide companionship.", Delivery: "Natural, present without intruding."}},
		{Name: "search", Category: "learning", NightSafe: true, OutcomeType: "action", Source: "proactive",
			WeightSocial: 0.05, WeightCare: 0.05, WeightCurious: 0.45, WeightQuiet: -0.10, WeightExplore: 0.30,
			SkillCard: types.SkillCard{Trigger: "Knowledge gap or active inquiry.", Action: "Search web for information.", Delivery: "Silent — results stored for later."}},
		{Name: "observe", Category: "learning", NightSafe: true, OutcomeType: "silent", Source: "proactive",
			WeightSocial: 0.10, WeightCare: 0.00, WeightCurious: 0.30, WeightQuiet: 0.00, WeightExplore: 0.60,
			SkillCard: types.SkillCard{Trigger: "Time since last observation.", Action: "Analyze user's screen.", Delivery: "Silent — feeds feature computation."}},
		{Name: "reflect", Category: "learning", NightSafe: true, OutcomeType: "silent", Source: "proactive",
			WeightSocial: 0.00, WeightCare: 0.00, WeightCurious: 0.00, WeightQuiet: 0.20, WeightExplore: 0.75,
			SkillCard: types.SkillCard{Trigger: "Time for strategic reflection.", Action: "Distill principles from outcomes.", Delivery: "Silent — stored in L3 memory."}},
		{Name: "analyze_patterns", Category: "learning", NightSafe: true, OutcomeType: "silent", Source: "proactive",
			WeightSocial: 0.00, WeightCare: 0.00, WeightCurious: 0.20, WeightQuiet: 0.00, WeightExplore: 0.65,
			SkillCard: types.SkillCard{Trigger: "Unanalyzed outcome patterns.", Action: "Find action×time patterns.", Delivery: "Silent."}},
		{Name: "care_quiet", Category: "care", NightSafe: true, OutcomeType: "silent", Source: "proactive",
			WeightSocial: 0.05, WeightCare: 0.40, WeightCurious: 0.00, WeightQuiet: 0.50, WeightExplore: 0.00,
			SkillCard: types.SkillCard{Trigger: "User needs quiet.", Action: "Record concern silently.", Delivery: "Silent — emotional state update only."}},
		{Name: "greet_return", Category: "social", NightSafe: false, OutcomeType: "speak", Source: "proactive",
			WeightSocial: 0.70, WeightCare: 0.20, WeightCurious: 0.10, WeightQuiet: -0.10, WeightExplore: 0.00,
			SkillCard: types.SkillCard{Trigger: "User returned after >1h away.", Action: "Warm welcome back.", Delivery: "Happy and energetic."}},
		{Name: "none", Category: "none", NightSafe: true, OutcomeType: "silent", Source: "proactive",
			WeightSocial: 0.00, WeightCare: 0.00, WeightCurious: 0.00, WeightQuiet: 1.00, WeightExplore: 0.00,
			SkillCard: types.SkillCard{Trigger: "No suitable action.", Action: "Do nothing this tick.", Delivery: "N/A"}},
	}
}

// ActionByName looks up an action by name. Returns nil if not found.
func ActionByName(name string) *types.ActionDef {
	for _, a := range BuildActions() {
		if a.Name == name {
			return &a
		}
	}
	return nil
}

// FilterNightSafe returns only actions safe to use at night.
// Always includes "none" so the system can choose silence.
func FilterNightSafe(scored []types.ScoredAction) []types.ScoredAction {
	out := make([]types.ScoredAction, 0, len(scored))
	for _, s := range scored {
		if s.Action.NightSafe || s.Action.Name == "none" {
			out = append(out, s)
		}
	}
	return out
}

// NightSafeActionNames returns the sorted names of NightSafe actions for prompt display.
func NightSafeActionNames() []string {
	var names []string
	for _, a := range BuildActions() {
		if a.NightSafe {
			names = append(names, a.Name)
		}
	}
	return names
}

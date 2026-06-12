package types

// ── Entity & Relation Types ──

// EntityKind classifies whose perspective a memory entry is about.
type EntityKind string

const (
	KindUser         EntityKind = "user"
	KindCharacter    EntityKind = "character"
	KindRelationship EntityKind = "relationship"
)

// RelationType constrains the semantics of a memory entry.
type RelationType string

const (
	// ── user kind — facts about a human ──
	RelPreference RelationType = "preference" // 偏好/喜好
	RelTrait      RelationType = "trait"      // 性格/特征
	RelHabit      RelationType = "habit"      // 习惯/日常
	RelIdentity   RelationType = "identity"   // 身份/背景
	RelEmotional  RelationType = "emotional"  // 情感状态
	RelBoundary   RelationType = "boundary"   // 边界/禁忌

	// ── user kind — 行为模式 (SourceObserved, 从屏幕/行为中自动提取) ──
	RelWorkPattern  RelationType = "work_pattern"  // 工作模式
	RelSleepPattern RelationType = "sleep_pattern" // 作息模式
	RelCommStyle    RelationType = "comm_style"    // 沟通风格
	RelFocusArea    RelationType = "focus_area"    // 当前关注领域
	RelTechLevel    RelationType = "tech_level"    // 技术水平

	// ── character kind — AI self-model ──
	RelSelfAwareness RelationType = "self_awareness" // 自我认知
	RelLearned       RelationType = "learned"        // 习得行为
	RelRoleNote      RelationType = "role_note"      // 角色备注

	// ── relationship kind — between user and AI ──
	RelDynamic      RelationType = "dynamic"       // 互动模式
	RelMilestone    RelationType = "milestone"     // 关系里程碑
	RelTension      RelationType = "tension"       // 摩擦/冲突
	RelSharedMemory RelationType = "shared_memory" // 共同记忆
	RelAgreement    RelationType = "agreement"     // 约定/共识
)

// AllowedRelations returns valid relation types for a given entity kind.
func AllowedRelations(kind EntityKind) []RelationType {
	switch kind {
	case KindUser:
		return []RelationType{RelPreference, RelTrait, RelHabit, RelIdentity, RelEmotional, RelBoundary,
			RelWorkPattern, RelSleepPattern, RelCommStyle, RelFocusArea, RelTechLevel}
	case KindCharacter:
		return []RelationType{RelSelfAwareness, RelLearned, RelRoleNote}
	case KindRelationship:
		return []RelationType{RelDynamic, RelMilestone, RelTension, RelSharedMemory, RelAgreement}
	default:
		return nil
	}
}

// IsValidRelation checks if relation is valid for the given entity kind.
func IsValidRelation(entityKind EntityKind, rel RelationType) bool {
	for _, allowed := range AllowedRelations(entityKind) {
		if rel == allowed {
			return true
		}
	}
	return false
}

// ── Memory Cell Types ──

// MemCellType classifies a memory by its cognitive role.
type MemCellType string

const (
	CellFact     MemCellType = "fact"     // 事实
	CellPrefer   MemCellType = "prefer"   // 偏好
	CellEvent    MemCellType = "event"    // 事件
	CellEmotion  MemCellType = "emotion"  // 情绪时刻
	CellSkill    MemCellType = "skill"    // 技能
	CellRelation MemCellType = "relation" // 关系
)

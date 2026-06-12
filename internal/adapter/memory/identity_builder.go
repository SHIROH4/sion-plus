package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/SHIROH4/sion-plus/internal/port"
)

// IdentityBuilder synthesizes SelfModel narratives from promoted reflections.
type IdentityBuilder struct {
	store    *SelfModelStore
	executor port.LLMExecutor
	db       *SQLiteStore
	lastRun  time.Time
}

func NewIdentityBuilder(store *SelfModelStore, executor port.LLMExecutor, db *SQLiteStore) *IdentityBuilder {
	return &IdentityBuilder{store: store, executor: executor, db: db}
}

// BuildIdentity collects promoted reflections and asks LLM to update the SelfModel.
func (b *IdentityBuilder) BuildIdentity(ctx context.Context) error {
	if b.executor == nil {
		return fmt.Errorf("no LLM executor")
	}

	// Get all promoted reflections
	refs, err := b.db.ListReflectionsByStatus(ctx, []string{"promoted"}, 0)
	if err != nil || len(refs) == 0 {
		return fmt.Errorf("no promoted reflections")
	}

	// Separate by entity
	var masterTraits, sionTraits, relationshipTraits []string
	for _, r := range refs {
		switch r.Entity {
		case "master":
			masterTraits = append(masterTraits, r.Text)
		case "sion", "neko":
			sionTraits = append(sionTraits, r.Text)
		default:
			relationshipTraits = append(relationshipTraits, r.Text)
		}
	}

	// Load current model
	current, _ := b.store.Load(ctx)

	// Build new UserModel
	if len(masterTraits) > 0 {
		userModel, err := b.generateModel(ctx, "用户画像", current.UserModel, masterTraits)
		if err == nil && userModel != "" {
			current.UserModel = userModel
		}
	}

	// Build new SelfModel
	if len(sionTraits) > 0 {
		selfModel, err := b.generateModel(ctx, "Sion的自我认知", current.SelfModel, sionTraits)
		if err == nil && selfModel != "" {
			current.SelfModel = selfModel
		}
	}

	// If we have relationship traits, append to self model
	if len(relationshipTraits) > 0 {
		relBlock := "互动模式:\n" + strings.Join(relationshipTraits, "\n")
		if current.SelfModel != "" {
			current.SelfModel += "\n\n" + relBlock
		} else {
			current.SelfModel = relBlock
		}
	}

	b.lastRun = time.Now()
	log.Printf("[IdentityBuilder] built identity: user=%d traits, self=%d traits, rel=%d traits",
		len(masterTraits), len(sionTraits), len(relationshipTraits))

	return b.store.Save(ctx, current)
}

func (b *IdentityBuilder) generateModel(ctx context.Context, label, current string, traits []string) (string, error) {
	prompt := fmt.Sprintf(identityBuildPrompt, label, current, strings.Join(traits, "\n- "))

	resp, err := b.executor.Chat(ctx, "", []port.LLMMessage{
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return "", err
	}

	var result struct {
		Narrative string `json:"narrative"`
	}
	if err := json.Unmarshal([]byte(extractJSON(resp)), &result); err != nil {
		// If JSON parse fails, use raw response
		return strings.TrimSpace(resp), nil
	}
	return result.Narrative, nil
}

// ── Prompt ─────────────────────────────────────────────────────────

const identityBuildPrompt = `你是 Sion 的记忆整合器。基于已验证的事实片段，更新 %s。

当前版本：
%s

新的事实片段：
- %s

请将这些新信息整合到现有描述中。要求：
1. 保持自然叙事风格（像人物档案），2-3 句话
2. 如果新信息与旧信息不冲突，直接补充
3. 如果新信息更新了旧信息（如技术栈变化），替换旧信息
4. 不要编造不在事实片段中的内容
5. 使用中文

返回纯JSON：{"narrative": "..."}`

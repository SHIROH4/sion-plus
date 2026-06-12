package memory

import (
	"fmt"
	"strings"
)

// Language selects the prompt language.
type Language string

const (
	LangZH Language = "zh"
	LangEN Language = "en"
)

// ── Fact Extraction Prompt ─────────────────────────────────────────
// Based on NEKO's FACT_EXTRACTION_PROMPT plus few-shot examples.

var factExtractionPrompts = map[Language]string{
	LangZH: `你是一个记忆提取器。从以下对话中提取关于用户的原子化事实。

规则:
1. 每条事实只包含一个信息点。不要合并多个事实。
2. 按 entity 分类:
   - "master": 关于用户的事实 (偏好、习惯、身份、技能、边界)
   - "neko": 关于你自己的事实 (用户如何称呼你、对你的要求)
   - "relationship": 关于你们关系的事实 (互动模式、约定、里程碑)
3. 按 relation_type 分类:
   - preference: 用户喜欢/不喜欢什么
   - trait: 用户性格特征
   - habit: 用户习惯/日常
   - identity: 用户身份/背景
   - emotional: 当前情感状态
   - boundary: 边界/禁忌 (用户明确说不要做、不要聊的)
   - work_pattern: 工作模式
   - tech_level: 技术水平
   - comm_style: 沟通风格偏好
4. 按 source_tier 标记:
   - "explicit": 用户亲口说的 ("我喜欢Rust")
   - "observed": 从行为中观察到的 (连续5天用VS Code)
   - "inferred": 从上下文中推理的 (多个信号 → 一个模式)
5. importance 评分 (1-10):
   - 10: 用户明确说"记住这个" / 边界类 / 核心身份
   - 7-9: 强偏好 / 明确的习惯 / 重要身份信息
   - 5-6: 一般偏好 / 普通信息
   - 3-4: 可能只是临时提及
   - 1-2: 闲聊级 (直接跳过，不提取)
6. 不要提取已经很明显的事实 (如"用户是男性"、"用户会说中文")。
7. 如果对话中没有值得提取的新信息，返回空数组。

Few-shot 示例:
对话: 用户:"我最近在学Rust，感觉比C++舒服多了" AI:"Rust确实很适合系统编程"
→ {"entity": "master", "relation_type": "preference", "content": "觉得Rust比C++舒服", "source_tier": "explicit", "importance": 7}
→ {"entity": "master", "relation_type": "tech_level", "content": "正在学习Rust", "source_tier": "explicit", "importance": 5}

对话: 用户:"别在我debug的时候打扰我" AI:"明白了，以后注意"
→ {"entity": "master", "relation_type": "boundary", "content": "debug时不要被打扰", "source_tier": "explicit", "importance": 10}

对话: 用户:"今天好累" AI:"注意休息"
→ {} (临时情感表达，不是持久事实)

对话:
%s

6. 按 memcell_type 标记记忆认知类型:
   - "fact": 客观事实 (用户身份、技能水平等)
   - "prefer": 偏好 (喜欢/不喜欢)
   - "event": 一次性事件
   - "emotion": 情绪时刻
   - "skill": 技能
   - "relation": 关系动态

返回JSON: {"facts": [{"entity": "...", "relation_type": "...", "content": "...", "source_tier": "...", "importance": N, "memcell_type": "..."}]}`,

	LangEN: `You are a memory extractor. Extract atomic facts about the user from the conversation below.

Rules:
1. Each fact contains exactly ONE piece of information.
2. entity classification:
   - "master": facts about the user (preferences, habits, identity, skills, boundaries)
   - "neko": facts about yourself (how the user addresses you, requirements)
   - "relationship": facts about your relationship (interaction patterns, agreements)
3. relation_type classification:
   - preference: what the user likes/dislikes
   - trait: personality traits
   - habit: routines/habits
   - identity: background/role
   - emotional: emotional state
   - boundary: explicit prohibitions (user saying "don't X / stop Y")
   - work_pattern: work patterns
   - tech_level: technical skill level
   - comm_style: communication style preference
4. source_tier:
   - "explicit": user said it directly ("I like Rust")
   - "observed": inferred from behavior (VS Code for 5 consecutive days)
   - "inferred": LLM inference from multiple signals
5. importance (1-10):
   - 10: user explicitly said "remember this" / boundaries / core identity
   - 7-9: strong preferences / clear habits
   - 5-6: general information
   - 3-4: possibly temporary
   - 1-2: skip entirely
6. If nothing worth extracting, return empty array.

Few-shot examples:
User: "I'm learning Rust, feels much better than C++"
→ {"entity": "master", "relation_type": "preference", "content": "prefers Rust over C++", "source_tier": "explicit", "importance": 7}

User: "Don't interrupt me when I'm debugging"
→ {"entity": "master", "relation_type": "boundary", "content": "do not interrupt during debugging", "source_tier": "explicit", "importance": 10}

Conversation:
%s

Return JSON: {"facts": [{"entity": "...", "relation_type": "...", "content": "...", "source_tier": "...", "importance": N}]}`,
}

func buildFactExtractionPrompt(messages string) string {
	return fmt.Sprintf(factExtractionPrompts[LangZH], messages)
}

func buildFactExtractionPromptLang(lang Language, messages string) string {
	p, ok := factExtractionPrompts[lang]
	if !ok {
		p = factExtractionPrompts[LangZH]
	}
	return fmt.Sprintf(p, messages)
}

// ── Signal Detection Prompt ────────────────────────────────────────

var signalDetectionPrompts = map[Language]string{
	LangZH: `你是一个信号检测器。检测新提取的事实和已有事实之间的关系。

对每一对(new_fact, existing_fact)判断:
- "reinforce": 新事实确认/强化已有事实
- "contradict": 新事实与已有事实直接矛盾
- null: 无关

重要规则 (避免误判):
1. 临时状态 vs 长期偏好不是矛盾:
   "今天不想喝咖啡" ≠ 矛盾 "喜欢咖啡" → null
2. 方式变了偏好没变不是矛盾:
   "改喝手冲了" ≠ 矛盾 "喜欢咖啡" → null (甚至可能是 reinforce)
3. 情绪表达不是矛盾:
   "喝多了难受" ≠ 矛盾 "喜欢啤酒" → null
4. 新增限制条件不是矛盾:
   "肝不好不能喝酒" + "喜欢啤酒" → contradict (这是真正的矛盾)
5. 只有语义上真正互斥时才标记 contradict。

Few-shot:
已有: [ID:1] "喜欢咖啡" 新: "每天都喝两杯咖啡" → reinforce
已有: [ID:2] "是后端工程师" 新: "最近开始写前端了" → null (新增技能，不是推翻)
已有: [ID:3] "喜欢安静" 新: "医生说不能喝咖啡了" → null (无关话题)
已有: [ID:4] "是Go开发者" 新: "不用Go了，转Rust了" → contradict

新提取的事实:
%s

已有事实 (只关注与新事实相关的):
%s

返回JSON: {"signals": [{"entry_id": 已有事实ID, "type": "reinforce|contradict", "source_fact_content": "触发的新事实内容"}]}
无信号时返回 {"signals": []}`,

	LangEN: `You are a signal detector. Detect relationships between newly extracted facts and existing facts.

For each pair (new_fact, existing_fact) classify as:
- "reinforce": new fact confirms/strengthens existing fact
- "contradict": new fact directly contradicts existing fact
- null: unrelated

Key rules:
1. Temporary state ≠ contradiction: "don't want coffee today" ≠ contradicts "likes coffee"
2. Method change ≠ contradiction: "switched to pour-over" ≠ contradicts "likes coffee"
3. Emotional expression ≠ contradiction: "drank too much, feeling sick" ≠ contradicts "likes beer"
4. Only mark contradict when semantically mutually exclusive.

Few-shot:
Existing: [ID:1] "likes coffee" New: "drinks two cups daily" → reinforce
Existing: [ID:2] "backend engineer" New: "started learning frontend" → null (not contradictory)
Existing: [ID:4] "Go developer" New: "stopped using Go, switched to Rust" → contradict

New facts:
%s

Existing facts:
%s

Return JSON: {"signals": [{"entry_id": N, "type": "reinforce|contradict", "source_fact_content": "..."}]}
No signals: {"signals": []}`,
}

func buildSignalDetectionPrompt(newFacts, existingFacts string) string {
	return fmt.Sprintf(signalDetectionPrompts[LangZH], newFacts, existingFacts)
}

// ── Reflection + Diary Prompt ──────────────────────────────────────

var reflectAndDiaryPrompts = map[Language]string{
	LangZH: `你是一个反思引擎。做两件事:
1. 将多条碎片化事实合成为更高级的反思洞察
2. 为最近的时间段生成一篇简短的日记

未吸收的事实 (尚未被反思过的):
%s

最近的对话:
%s

反思规则:
- 将 2-5 条相关事实聚合为一个洞察
- 反思不是事实的罗列，而是提炼出的模式、矛盾或深层偏好
- entity 应为 "master" (关于用户), "neko" (关于Sion自己), 或 "relationship" (关于关系)
- 每条反思的 importance 取源事实中最高的那个

日记规则:
- title: 10字以内的简短标题
- summary: 2-3句话概括这段时间的主要事件和氛围
- emotion_valence: -1.0(非常负面) 到 1.0(非常正面)
- emotion_arousal: -1.0(非常平静) 到 1.0(非常激动)
- emotion_primary: "happy"|"sad"|"angry"|"anxious"|"calm"|"excited"|"focused"|"tired"

返回JSON:
{
  "reflections": [{"text": "...", "entity": "master", "relation_type": "preference", "importance": 7}],
  "diary": {"title": "...", "summary": "...", "emotion_valence": 0.3, "emotion_arousal": -0.2, "emotion_primary": "focused"}
}
无足够材料时 reflections 返回空数组。`,

	LangEN: `You are a reflection engine. Do two things:
1. Synthesize multiple fragmented facts into higher-level insights
2. Generate a short diary entry for the recent time window

Unabsorbed facts:
%s

Recent conversation:
%s

Reflection rules:
- Aggregate 2-5 related facts into one insight
- Reflections are patterns/contradictions/deep preferences, not fact lists
- entity should be "master" (about user), "neko" (about Sion herself), or "relationship"
- importance = max of source facts

Diary rules:
- title: short title (max 10 words)
- summary: 2-3 sentences capturing the period's events and mood
- emotion_valence: -1.0 (very negative) to 1.0 (very positive)
- emotion_arousal: -1.0 (very calm) to 1.0 (very excited)
- emotion_primary: "happy"|"sad"|"angry"|"anxious"|"calm"|"excited"|"focused"|"tired"

Return JSON with "reflections" array and "diary" object. Empty reflections if insufficient material.`,
}

func buildReflectAndDiaryPrompt(unabsorbedFacts, recentMessages string) string {
	return fmt.Sprintf(reflectAndDiaryPrompts[LangZH], unabsorbedFacts, recentMessages)
}

// ── Compression Prompt ─────────────────────────────────────────────

var compressionPrompts = map[Language]string{
	LangZH: `请总结以下对话内容，生成简洁但信息丰富的摘要:

======以下为对话======
%s
======以上为对话======

规则:
1. 保留关键信息、重要事实和主要讨论点，不能有误导性
2. 保留用户的负面反馈 (明确说"别再提X / 不要做Y / 不想聊Z")，原样记录
3. 如果有事实被纠正，保留纠正过程 ("原以为X，后被纠正为Y")
4. 避免过度重复相同词汇，首次提及后用代词替换
5. 摘要不超过200字

返回JSON: {"summary": "摘要内容"}`,

	LangEN: `Summarize the following conversation into a concise yet informative summary:

======以下为对话======
%s
======以上为对话======

Rules:
1. Preserve key information, important facts, and main discussion points
2. Preserve user negative feedback verbatim ("don't mention X / stop discussing Y")
3. If facts were corrected, keep the correction trajectory ("originally X, later corrected to Y")
4. Avoid excessive repetition; use pronouns after first mention
5. Summary max 200 words

Return JSON: {"summary": "..."}`,
}

func buildCompressionPrompt(messages string) string {
	return fmt.Sprintf(compressionPrompts[LangZH], messages)
}

// ── Formatting helpers ─────────────────────────────────────────────

func formatMessagesForPrompt(msgs []string) string {
	return strings.Join(msgs, "\n")
}

func formatFactsForPrompt(contents []string, ids []int64) string {
	var b strings.Builder
	for i, c := range contents {
		b.WriteString(fmt.Sprintf("[ID:%d] %s\n", ids[i], c))
	}
	return b.String()
}

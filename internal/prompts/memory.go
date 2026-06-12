package prompts

// ── Fact Extraction ──

// PromptFactExtraction is the prompt for extracting atomic facts from conversation.
// Placeholder: %s = formatted dialogue turns
const PromptFactExtraction = `## 事实提取

从以下对话中提取关于用户的原子事实。每条事实必须是一条独立的、可验证的信息。

### 对话记录
%s

### 提取规则
1. 提取具体的事实，格式为 JSON 对象数组
2. 每条事实包含：
   - entity: "master" | "neko" | "relationship"
   - relation_type: preference/trait/habit/identity/emotional/boundary (对master)
                    self_awareness/learned/role_note (对neko)
                    dynamic/milestone/tension/shared_memory/agreement (对relationship)
   - content: 事实文本
   - importance: 1-10 (10=核心身份信息，1=短暂提及)
   - memcell_type: fact/prefer/event/emotion/skill/relation
3. 不要提取纯技术讨论中的临时信息
4. 不要提取情绪波动（日记会单独记录情绪）
5. 如果对话中没有新的事实，输出空数组 []

### 输出格式
[
  {"entity":"master","relation_type":"preference","content":"主人喜欢用深色主题","importance":6,"memcell_type":"prefer"},
  {"entity":"master","relation_type":"identity","content":"主人是后端工程师","importance":9,"memcell_type":"fact"}
]

只输出 JSON 数组。`

// ── Signal Detection ──

// PromptSignalDetection is the prompt for checking new facts against existing ones.
// Placeholders: %s[1] = new facts list, %s[2] = existing facts list
const PromptSignalDetection = `## 信号检测

检查新提取的事实与已有事实之间的关系。

### 新提取的事实
%s

### 已有的事实
%s

### 任务
对每对新事实×旧事实，判断它们之间的关系：
- "reinforce": 新事实确认/强化了旧事实（用户又提到了同样的偏好）
- "contradict": 新事实与旧事实矛盾（用户说法变了）
- "unrelated": 两条事实无关

### 输出格式
[
  {"new_fact_idx": 0, "old_fact_id": 42, "relation": "reinforce", "confidence": 0.8},
  {"new_fact_idx": 1, "old_fact_id": 15, "relation": "contradict", "confidence": 0.9}
]

只输出 JSON 数组。没有关联就输出 []。`

// ── Diary ──

// PromptDiaryGeneration generates an episodic diary entry.
// Placeholders: %s[1] = recent turns, %s[2] = current emotion
const PromptDiaryGeneration = `## 日记生成

根据最近对话，生成一条简短的日记条目。

### 最近对话
%s

### 当前情绪
%s

### 输出格式
{
  "title": "简短的标题（10字以内）",
  "summary": "2-3句话的日记摘要",
  "emotion_valence": -1到1,
  "emotion_arousal": -1到1,
  "emotion_primary": "joy/sadness/anger/fear/surprise/neutral"
}

只输出 JSON。`

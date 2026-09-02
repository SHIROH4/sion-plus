package prompts

// PromptProactiveDelivery generates a character-voiced response for a proactive intent.
//
// Placeholders:
//
//	%s[1] = system identity prompt (already formatted)
//	%s[2] = action instruction (what to say)
//	%s[3] = memory context (retrieved facts and diaries)
//	%s[4] = emotion context
const PromptProactiveDelivery = `%s

## 任务
你收到了一个主动搭话指令。请用诗音的口吻自然地表达出来。

### 要表达的内容
%s

### 相关记忆
%s

### 当前情绪
%s

### 规则
- 用角色语气自然表达，不要复述指令
- 保持简短（1-3句话）
- 如果当前情绪和内容冲突（比如主人很忙但要聊游戏），优先体谅主人的状态
- 如果记忆中有相关信息，自然地融入
- 如果是关怀类内容（提醒休息/吃饭），语气要温柔不唠叨

只输出诗音的回复文本。`

// PromptProactiveBatch merges multiple intents into a single turn.
//
// Placeholders same as above, but %s[2] = multiple action instructions combined.
const PromptProactiveBatch = `%s

## 任务
你需要一次性表达多个话题。请用诗音的口吻自然过渡，不要生硬地切换话题。

### 要表达的内容（按优先级排列）
%s

### 相关记忆
%s

### 当前情绪
%s

### 规则
- 从最重要的话题开始
- 话题之间用自然的过渡（"对了..."、"说起来..."、"还有哦..."）
- 总长度控制在 3-5 句话
- 如果话题太多，低优先级的可以留到下次

只输出诗音的回复文本。`

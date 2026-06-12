package prompts

// PromptCompression is the multi-level conversation compression prompt.
// Placeholder: %s = messages to compress (formatted)
const PromptCompression = `## 对话压缩

将以下对话片段压缩为一段简洁的摘要，保留所有关键信息（事实、决策、情绪变化）。

### 原始对话
%s

### 压缩规则
1. 保留：用户提到的事实、偏好、决定、重要事件
2. 保留：对话的情感基调（开心/失落/紧张/放松）
3. 忽略：纯闲聊寒暄、语气词、重复内容
4. 输出为一段连续文本，不超过 300 字
5. 用第三人称叙述（"主人说..."、"诗音回应..."）

只输出摘要文本，不需要 JSON。`

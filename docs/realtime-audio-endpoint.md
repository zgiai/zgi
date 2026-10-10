# Realtime 实时语音推理端点（调研与现状）

> 状态：**仅调研记录，未实现**。价格 schema、必填校验、音频 token 采集、定价引擎报价已就绪（见下）。
> 记录时间：基于当前仓库 `zgi-pre` / `zgi-console`。

## 1. 它是什么

不是普通 HTTP 一问一答接口，而是**长期保持的双向会话式推理服务**：客户端与服务端建立持久双向连接（通常是 WebSocket），双方持续互发事件与音频流。用户边说，模型边听、边理解、边回语音，并支持实时打断。

## 2. 与现有端点的差异

现有链路：`transcription`(STT) → `chat`(LLM) → `speech`(TTS)，串行三段，延迟高、不能打断、丢语气信息。
Realtime 把三段合并为一个会话：模型直接吃音频、直接吐音频，全双工、可打断、边生成边播。

## 3. 协议形态（以 OpenAI Realtime 为代表）

1. WebSocket 连接 `/v1/realtime`，URL/Header 带 model 与鉴权。
2. `session.update` 配置模型、voice、system prompt、工具、音频格式。
3. 客户端持续发 `input_audio_buffer.append`（base64 PCM16 分片）。
4. 服务端 VAD 事件：`input_audio_buffer.speech_started` / `speech_stopped`。
5. `response.create` 触发回答，服务端回 `response.created`、`response.audio.delta`、`response.audio.done`、`response.audio_transcript.delta`、`response.done`（带 usage）。
6. 可打断：`response.cancel`。
7. 结束会话。

关键事件类别：session（created/updated）、conversation.item.*、input_audio_buffer.*、response.*、response.audio.*、error。

## 4. 计费

上游 usage 区分普通 token 与音频 token：
- `input_tokens_details.audio_tokens`（音频输入）
- `output_tokens_details.audio_tokens`（音频输出）

单价远高于文本（OpenAI 量级：音频输入约 $32/1M、音频输出约 $64/1M）。因此模型配置可切换「按文本 token」或「按音频 token」，后者用 `realtime_audio` 规则（meters `input_token`/`output_token`）表达。

不同厂商口径不同：OpenAI 系按音频 token；阿里百炼部分实时语音按输入音频秒数/时长；火山/Gemini 各有实现。

## 5. Pre 现状与已就绪部分

Pre **目前没有任何 realtime 推理端点**，只有模型的 `realtime` 能力标记；全仓库唯一 WebSocket 是豆包转写适配器（单向 STT 流），不是双向会话。

已就绪（本轮已完成）：
- 模型配置：`custom-model-dialog` 支持「按文本 token / 按音频 token」切换，写 `realtime_audio` 规则。
- 必填校验：`model-pricing-requirements.ts` 与后端 `pricing_missing.go` 模式感知。
- 运行时采集：`adapter.Usage.AudioInputTokens / AudioOutputTokens`，raw protocol 解析 `audio_tokens`。
- 报价：`pricing_engine.go` 新增 `PricingOperationRealtimeAudio` 与 `quoteRealtimeAudioTokens`，从模型 `pricing.rules` 解析音频 in/out 价并报价，缺规则时 fail-closed。测试：`pricing_engine_test.go`。

## 6. 实现建议

1. 传输：新建 WebSocket 网关（升级握手、鉴权、按现有 channel router 选路）。
2. 会话生命周期：创建/保持/超时/重连、并发上限、限流。
3. 事件透传/改写：注入/替换 API Key、模型名、system prompt；把上游事件转发回客户端。
4. 音频适配：PCM16/采样率/声道、base64 分片、VAD。
5. 计费：会话级累计音频 in/out token；结算时以 `PricingModelRef.Operation = PricingOperationRealtimeAudio` 调 `QuoteTokens`（音频 token 作为 prompt/completion），写 `llm_usage_bills`。
6. 可观测性：会话 ID、断线原因、错误事件、账单归因。

## 7. 参考

- OpenAI Realtime Events: https://mintlify.wiki/openai/openai-python/api/realtime/events
- Azure GPT Realtime via WebSockets: https://learn.microsoft.com/azure/ai-foundry/openai/how-to/realtime-audio-websockets
- Gemini Live API: https://ai.google.dev/gemini-api/docs/live-api
- 火山引擎豆包「端到端实时语音-全双工版本」: https://docs.volcengine.com/docs/6561/2549778
- 阿里百炼实时语音计费（按输入音频时长）: https://www.alibabacloud.com/help/en/model-studio/model-pricing

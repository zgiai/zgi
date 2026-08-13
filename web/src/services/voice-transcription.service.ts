import { http, webappHttp } from '@/lib/http';
import type { ApiResponseData } from '@/services/types/common';

interface VoiceTranscriptionResult {
  request_id: string;
  text: string;
}

interface VoiceTranscriptionClientError extends Error {
  code: 'INVALID_VOICE_TARGET' | 'INVALID_VOICE_AUDIO' | 'INVALID_TRANSCRIPTION_RESPONSE';
}

function voiceClientError(
  code: VoiceTranscriptionClientError['code'],
  message: string
): VoiceTranscriptionClientError {
  return Object.assign(new Error(message), { code });
}

function validateRequest(targetID: string, pcm: ArrayBuffer): string {
  const normalizedTargetID = targetID.trim();
  if (!normalizedTargetID) {
    throw voiceClientError('INVALID_VOICE_TARGET', 'A voice transcription target is required.');
  }
  if (pcm.byteLength === 0) {
    throw voiceClientError('INVALID_VOICE_AUDIO', 'PCM audio is required.');
  }
  return encodeURIComponent(normalizedTargetID);
}

function readTranscript(response: ApiResponseData<VoiceTranscriptionResult>): string {
  const transcript = response.data?.text?.trim();
  if (!transcript) {
    throw voiceClientError(
      'INVALID_TRANSCRIPTION_RESPONSE',
      'Voice transcription returned no text.'
    );
  }
  return transcript;
}

const requestConfig = (signal: AbortSignal) => ({
  headers: { 'Content-Type': 'audio/pcm' },
  retryAttemptsOverride: 0,
  signal,
  skipErrorHandling: true,
});

export async function transcribeAgentDraftVoice(
  agentID: string,
  pcm: ArrayBuffer,
  signal: AbortSignal
): Promise<string> {
  const targetID = validateRequest(agentID, pcm);
  const response = await http.post<ApiResponseData<VoiceTranscriptionResult>>(
    `/console/api/agents/${targetID}/runtime/audio/transcriptions`,
    pcm,
    requestConfig(signal)
  );
  return readTranscript(response);
}

export async function transcribeAgentWebAppVoice(
  webAppID: string,
  pcm: ArrayBuffer,
  signal: AbortSignal
): Promise<string> {
  const targetID = validateRequest(webAppID, pcm);
  const response = await webappHttp.post<ApiResponseData<VoiceTranscriptionResult>>(
    `/console/api/webapps/${targetID}/runtime/audio/transcriptions`,
    pcm,
    requestConfig(signal)
  );
  return readTranscript(response);
}

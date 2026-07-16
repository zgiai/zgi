import type {
  AIChatUserInputRequest,
  AIChatUserInputResponse,
} from '@/services/types/aichat';

export function buildOptimisticUserInputResponse(
  request: AIChatUserInputRequest | undefined,
  requestId: string,
  values: Record<string, string>,
  answeredAt = Math.floor(Date.now() / 1000)
): AIChatUserInputResponse | null {
  if (!request) return null;
  const answers = (request.questions ?? [])
    .map((question, index) => {
      const questionId = question.id?.trim() || `q${index + 1}`;
      return {
        question_id: questionId,
        question: question.question?.trim() ?? '',
        value: values[questionId]?.trim() ?? '',
      };
    })
    .filter(answer => answer.question && answer.value);
  if (answers.length === 0) return null;

  return {
    request_id: requestId,
    message: request.message?.trim(),
    status: 'answered',
    answers,
    answer_count: answers.length,
    answered_at: answeredAt,
    optimistic: true,
  };
}

export function upsertUserInputResponse(
  responses: AIChatUserInputResponse[] | undefined,
  response: AIChatUserInputResponse
): AIChatUserInputResponse[] {
  if (!response.request_id) return [...(responses ?? []), response];
  return [
    ...(responses ?? []).filter(item => item.request_id !== response.request_id),
    response,
  ];
}

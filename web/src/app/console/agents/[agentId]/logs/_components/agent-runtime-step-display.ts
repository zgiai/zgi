import type { AgentRuntimeStep } from '@/services/types/agent-runtime-log';

interface RuntimeLogTranslator {
  (key: never, params?: Record<string, string | number>): string;
  has?: (key: never) => boolean;
}

export interface AgentRuntimeStepDisplay {
  title: string;
  subtitle: string;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

function numberValue(value: unknown): number | null {
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  if (typeof value === 'string' && value.trim() !== '') {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : null;
  }
  return null;
}

function stepProcess(step: AgentRuntimeStep): Record<string, unknown> {
  return isRecord(step.process) ? step.process : {};
}

function fallbackName(value: string, fallback: string): string {
  return value || fallback;
}

function translate(
  t: RuntimeLogTranslator,
  key: string,
  params?: Record<string, string | number>
): string {
  return t(key as never, params);
}

function hasTranslation(t: RuntimeLogTranslator, key: string): boolean {
  return typeof t.has !== 'function' || t.has(key as never);
}

function localizedMapValue(
  t: RuntimeLogTranslator,
  mapKey: string,
  raw: string,
  fallback: string
): string {
  if (!raw) return fallback;
  const key = `${mapKey}.${raw}`;
  if (!hasTranslation(t, key)) {
    return fallback;
  }
  return translate(t, key);
}

function skillLabel(t: RuntimeLogTranslator, skillID: string): string {
  return localizedMapValue(t, 'appLogs.builtinSkills', skillID, skillID);
}

function toolLabel(t: RuntimeLogTranslator, toolName: string): string {
  return localizedMapValue(t, 'appLogs.builtinTools', toolName, toolName);
}

function phaseLabel(t: RuntimeLogTranslator, phase: string): string {
  return localizedMapValue(t, 'appLogs.runtimeModelPhases', phase, phase);
}

function modelCallSubtitle(t: RuntimeLogTranslator, process: Record<string, unknown>): string {
  const phase = stringValue(process.phase);
  const round = numberValue(process.round);
  if (phase === 'skill_planning' && round !== null && round >= 0) {
    return translate(t, 'appLogs.runtimeModelPhases.skill_planningRound', { round: round + 1 });
  }
  return phaseLabel(t, phase);
}

export function getAgentRuntimeStepDisplay(
  step: AgentRuntimeStep,
  t: RuntimeLogTranslator
): AgentRuntimeStepDisplay {
  const process = stepProcess(step);
  const type = stringValue(step.type);
  const rawTitle = stringValue(step.title);

  switch (type) {
    case 'model_call': {
      const model = fallbackName(
        stringValue(process.model),
        translate(t, 'appLogs.runtimeFallbacks.model')
      );
      return {
        title: translate(t, 'appLogs.runtimeEventTitles.modelCall', { name: model }),
        subtitle: modelCallSubtitle(t, process),
      };
    }
    case 'tool_call':
    case 'tool': {
      const rawToolName = stringValue(process.tool_name);
      const rawSkillID = stringValue(process.skill_id);
      const displayTool = fallbackName(
        toolLabel(t, rawToolName),
        translate(t, 'appLogs.runtimeFallbacks.tool')
      );
      const displaySkill = skillLabel(t, rawSkillID);
      return {
        title: translate(t, 'appLogs.runtimeEventTitles.toolCall', { name: displayTool }),
        subtitle:
          displaySkill && rawSkillID
            ? translate(t, 'appLogs.runtimeSubtitles.skillTool', {
                skill: displaySkill,
                tool: rawToolName || displayTool,
              })
            : rawToolName || displayTool,
      };
    }
    case 'skill_load':
    case 'skill': {
      const rawSkillID = stringValue(process.skill_id);
      const displaySkill = fallbackName(skillLabel(t, rawSkillID), rawTitle || type);
      return {
        title: translate(t, 'appLogs.runtimeEventTitles.skillLoad', { name: displaySkill }),
        subtitle: rawSkillID,
      };
    }
    case 'reference_read': {
      const path = fallbackName(
        stringValue(process.path),
        translate(t, 'appLogs.runtimeFallbacks.reference')
      );
      return {
        title: translate(t, 'appLogs.runtimeEventTitles.referenceRead', { name: path }),
        subtitle: skillLabel(t, stringValue(process.skill_id)),
      };
    }
    case 'intermediate_answer':
      return {
        title: translate(t, 'appLogs.runtimeEventTitles.intermediateAnswer'),
        subtitle: rawTitle || type,
      };
    case 'user_input_request':
      return {
        title: translate(t, 'appLogs.runtimeEventTitles.userInputRequest'),
        subtitle: rawTitle || type,
      };
    case 'guardrail':
      return {
        title: translate(t, 'appLogs.runtimeEventTitles.guardrail'),
        subtitle: rawTitle || type,
      };
    case 'model_answer':
      return {
        title: translate(t, 'appLogs.runtimeEventTitles.finalAnswer'),
        subtitle: type,
      };
    case 'user_input':
      return {
        title: translate(t, 'appLogs.runtimeUserInput'),
        subtitle: type,
      };
    default:
      return {
        title: rawTitle || type || translate(t, 'appLogs.runtimeFallbacks.event'),
        subtitle: type,
      };
  }
}

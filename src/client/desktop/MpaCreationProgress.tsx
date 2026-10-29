import { Check, Circle, LoaderCircle, Minus, Pause, X } from 'lucide-react';
import { LifecycleStage } from './LifecycleStatus';
import { useMessages } from './locale';
import type { MpaCreationTask } from './lifecycle-models';

export const creationStages = [
  'checking',
  'iam_role',
  'admin_workspace',
  'admin_database',
  'business_workspace',
  'network',
  'gateway',
  'worker',
  'openviking',
  'deploying',
  'verifying',
] as const;

export function normalizedStage(stage: string) {
  if (stage === 'openviking_key') return 'openviking';
  if (stage === 'database' || stage === 'skills') return 'deploying';
  return stage;
}

export type CreationStepState =
  | 'completed'
  | 'running'
  | 'pending'
  | 'skipped'
  | 'failed'
  | 'cancelled'
  | 'passed'
  | 'unknown';

export function creationStepStates(task: MpaCreationTask) {
  const current = normalizedStage(task.stage);
  const visited = new Set((task.visitedStages || []).map(normalizedStage));
  const hasHistory = Boolean(task.visitedStages?.includes('queued'));
  const currentIndex = creationStages.findIndex((stage) => stage === current);
  const furthest = Math.max(
    currentIndex,
    ...creationStages.map((stage, index) => (visited.has(stage) ? index : -1)),
  );
  return creationStages.map(
    (stage, index): { stage: string; state: CreationStepState } => {
      // Observations take precedence over configured skips, including resumed tasks.
      const optional =
        (stage === 'openviking' &&
          Boolean(task.openvikingMode) &&
          task.openvikingMode !== 'auto') ||
        (['admin_workspace', 'admin_database', 'business_workspace'].includes(
          stage,
        ) &&
          Boolean(task.pgHost));
      if (stage === current && task.state !== 'succeeded') {
        const state =
          task.state === 'failed'
            ? 'failed'
            : task.state === 'cancelled'
              ? 'cancelled'
              : task.state === 'running' || task.state === 'cancelling'
                ? 'running'
                : 'unknown';
        return { stage, state };
      }
      if (stage === current && task.state === 'succeeded')
        return { stage, state: 'completed' };
      if (visited.has(stage)) return { stage, state: 'completed' };
      if (optional) return { stage, state: 'skipped' };
      if (task.state === 'succeeded' || index < furthest)
        return { stage, state: hasHistory ? 'skipped' : 'passed' };
      return { stage, state: 'pending' };
    },
  );
}

export function MpaCreationProgress({ task }: { task: MpaCreationTask }) {
  const m = useMessages();
  const labels: Record<CreationStepState, string> = {
    completed: m.mpaStepCompleted(),
    running: m.mpaStepRunning(),
    pending: m.mpaStepPending(),
    skipped: m.mpaStepSkipped(),
    failed: m.mpaFailed(),
    cancelled: m.mpaCancelled(),
    passed: m.mpaStepPassed(),
    unknown: m.mpaUnknown(),
  };
  const icons = {
    completed: Check,
    running: LoaderCircle,
    pending: Circle,
    skipped: Minus,
    failed: X,
    cancelled: Pause,
    passed: Check,
    unknown: Circle,
  };
  return (
    <section
      className="mpa-creation-progress"
      aria-label={m.mpaCreationSteps()}
    >
      <h3>{m.mpaCreationSteps()}</h3>
      <ol className="mpa-creation-checklist">
        {creationStepStates(task).map(({ stage, state }) => {
          const Icon = icons[state];
          return (
            <li
              key={stage}
              data-stage={stage}
              data-state={state}
              aria-current={state === 'running' ? 'step' : undefined}
            >
              <Icon size={16} aria-hidden="true" />
              <LifecycleStage stage={stage} />
              <span className="mpa-step-state">{labels[state]}</span>
            </li>
          );
        })}
      </ol>
      {!task.visitedStages?.includes('queued') && (
        <p className="mpa-progress-note">{m.mpaStepsLegacyHint()}</p>
      )}
    </section>
  );
}

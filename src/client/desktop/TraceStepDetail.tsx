import { useMessages } from './locale';
import { X } from 'lucide-react';
import { IconButton } from '../ui';
import {
  formatTraceDuration,
  safeTraceDetails,
  traceCategoryLabel,
  traceStatusLabel,
  traceStepCategory,
  traceStepTitle,
  type TraceStep,
} from './trace-model';

export function TraceStepDetail({
  step,
  onClose,
}: {
  step: TraceStep;
  onClose: () => void;
}) {
  const m = useMessages();
  return (
    <aside className="trace-detail" aria-label={m.desktopTraceStepDetails()}>
      <header>
        <div>
          <span>
            {traceCategoryLabel(traceStepCategory(step), m)} · {step.kind}
          </span>
          <strong>{traceStepTitle(step)}</strong>
        </div>
        <IconButton
          className="trace-icon-button"
          label={m.desktopCloseStepDetails()}
          onClick={onClose}
        >
          <X size={14} />
        </IconButton>
      </header>
      <dl>
        <dt>{m.desktopStatus()}</dt>
        <dd className={step.status}>{traceStatusLabel(step.status, m)}</dd>
        <dt>{m.desktopDuration()}</dt>
        <dd>{formatTraceDuration(step.duration, m)}</dd>
        <dt>{m.desktopStarted()}</dt>
        <dd>{step.startedAt || m.valueUnavailable()}</dd>
        <dt>{m.desktopEnded()}</dt>
        <dd>
          {step.endedAt ||
            (step.status === 'running'
              ? m.desktopRunning()
              : m.valueUnavailable())}
        </dd>
        <dt>{m.desktopStepID()}</dt>
        <dd>{step.stepId}</dd>
        {step.sourceEventId && (
          <>
            <dt>{m.desktopSourceEvent()}</dt>
            <dd>{step.sourceEventId}</dd>
          </>
        )}
      </dl>
      {step.details && Object.keys(step.details).length > 0 && (
        <details>
          <summary>{m.desktopSafeDetails()}</summary>
          <pre>{JSON.stringify(safeTraceDetails(step.details), null, 2)}</pre>
        </details>
      )}
    </aside>
  );
}

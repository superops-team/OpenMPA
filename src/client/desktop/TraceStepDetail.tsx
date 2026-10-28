import { X } from 'lucide-react';
import { IconButton } from '../ui';
import {
  formatTraceDuration,
  safeTraceDetails,
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
  return (
    <aside className="trace-detail" aria-label="Trace step details">
      <header>
        <div>
          <span>
            {traceStepCategory(step)} · {step.kind}
          </span>
          <strong>{traceStepTitle(step)}</strong>
        </div>
        <IconButton
          className="trace-icon-button"
          label="Close step details"
          onClick={onClose}
        >
          <X size={14} />
        </IconButton>
      </header>
      <dl>
        <dt>Status</dt>
        <dd className={step.status}>{traceStatusLabel(step.status)}</dd>
        <dt>Duration</dt>
        <dd>{formatTraceDuration(step.duration)}</dd>
        <dt>Started</dt>
        <dd>{step.startedAt || 'Unavailable'}</dd>
        <dt>Ended</dt>
        <dd>
          {step.endedAt ||
            (step.status === 'running' ? 'Running' : 'Unavailable')}
        </dd>
        <dt>Step ID</dt>
        <dd>{step.stepId}</dd>
        {step.sourceEventId && (
          <>
            <dt>Source event</dt>
            <dd>{step.sourceEventId}</dd>
          </>
        )}
      </dl>
      {step.details && Object.keys(step.details).length > 0 && (
        <details>
          <summary>Safe details</summary>
          <pre>{JSON.stringify(safeTraceDetails(step.details), null, 2)}</pre>
        </details>
      )}
    </aside>
  );
}

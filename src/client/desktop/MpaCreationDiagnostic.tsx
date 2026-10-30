import { useState } from 'react';
import { Button } from '../ui';
import { lifecycleStageLabel } from './LifecycleStatus';
import { useMessages } from './locale';
import { safeMpaDiagnostic, mpaFailureReason } from './mpa-diagnostic';
import { mpaDiagnosticReasonLabel } from './mpa-diagnostic-labels';
import { normalizedStage } from './MpaCreationProgress';
import type { MpaCreationTask } from './lifecycle-models';

export function MpaCreationDiagnostic({ task }: { task: MpaCreationTask }) {
  const m = useMessages();
  const [copyState, setCopyState] = useState<'copied' | 'failed'>();
  const failed = task.state === 'failed';
  const succeeded = task.state === 'succeeded';
  const candidate = task.latestDiagnostic;
  const historical = Boolean(
    !failed &&
    candidate &&
    normalizedStage(candidate.stage) !== normalizedStage(task.stage),
  );
  // A retry from an earlier stage must never become the current failure reason.
  const diagnostic = failed
    ? candidate?.stage === task.stage && candidate.outcome === 'failed'
      ? candidate
      : undefined
    : candidate;
  const safe = safeMpaDiagnostic(diagnostic);
  if (succeeded || (!failed && !diagnostic)) return null;
  const reason = mpaDiagnosticReasonLabel(
    m,
    mpaFailureReason(task, diagnostic),
  );
  const fields: Array<[string, string]> = [
    [
      failed ? m.mpaFailureStage() : m.mpaDiagnosticStage(),
      lifecycleStageLabel(m, diagnostic?.stage || task.stage),
    ],
    ...(safe?.operation
      ? [
          [
            failed ? m.mpaFailureOperation() : m.mpaDiagnosticOperation(),
            safe.operation,
          ] as [string, string],
        ]
      : []),
    ...(safe?.errorCode
      ? [[m.mpaFailureErrorCode(), safe.errorCode] as [string, string]]
      : []),
    ...(safe?.exceptionType
      ? [[m.mpaFailureExceptionType(), safe.exceptionType] as [string, string]]
      : []),
    ...(safe?.httpStatus
      ? [
          [m.mpaFailureHttpStatus(), String(safe.httpStatus)] as [
            string,
            string,
          ],
        ]
      : []),
    ...(safe?.responseType
      ? [[m.mpaFailureResponseType(), safe.responseType] as [string, string]]
      : []),
    ...(safe?.requestId
      ? [[m.mpaFailureRequestId(), safe.requestId] as [string, string]]
      : []),
    ...(diagnostic &&
    Number.isInteger(diagnostic.attempt) &&
    diagnostic.attempt >= 1 &&
    diagnostic.attempt <= 8
      ? [
          [m.mpaDiagnosticAttempt(), String(diagnostic.attempt)] as [
            string,
            string,
          ],
        ]
      : []),
  ];
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(
        [
          ...(historical ? [m.mpaHistoricalDiagnosticHint()] : []),
          reason,
          ...fields.map(([label, value]) => `${label}: ${value}`),
        ].join('\n'),
      );
      setCopyState('copied');
    } catch {
      setCopyState('failed');
    }
  };
  const details = (
    <details
      key={historical ? 'history' : 'diagnostic'}
      className="mpa-diagnostic-details"
    >
      <summary>
        {historical
          ? m.mpaHistoricalDiagnostic()
          : failed
            ? m.mpaFailureDetails()
            : m.mpaSafeDiagnostic()}
      </summary>
      {historical && <p>{m.mpaHistoricalDiagnosticHint()}</p>}
      {!failed && <p>{reason}</p>}
      {!failed && !historical && diagnostic?.outcome === 'retrying' && (
        <p>{m.mpaDiagnosticRetrying()}</p>
      )}
      <dl>
        {fields.map(([label, value]) => (
          <div key={label}>
            <dt>{label}</dt>
            <dd>{value}</dd>
          </div>
        ))}
      </dl>
      <Button variant="outline" onClick={() => void copy()}>
        {m.mpaFailureCopy()}
      </Button>
      {copyState && (
        <p role="status">
          {copyState === 'copied'
            ? m.mpaFailureCopied()
            : m.mpaFailureCopyFailed()}
        </p>
      )}
    </details>
  );
  return failed ? (
    <div className="mpa-creation-failure" role="alert">
      <p>{reason}</p>
      {details}
    </div>
  ) : (
    details
  );
}

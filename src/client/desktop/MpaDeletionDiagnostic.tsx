import { useState } from 'react';
import { Button } from '../ui';
import { useMessages } from './locale';
import { safeMpaDiagnostic } from './mpa-diagnostic';
import { mpaDiagnosticReasonLabel } from './mpa-diagnostic-labels';
import type { Diagnostic } from './generated/mygo';

export type DeletionDiagnostic = Diagnostic & {
  reason?: string;
  source?: string;
  resourceHost?: string;
};
export function deletionErrorDiagnostic(
  error: unknown,
): DeletionDiagnostic | undefined {
  if (!error || typeof error !== 'object' || !('diagnostic' in error))
    return undefined;
  const value = error.diagnostic;
  if (!value || typeof value !== 'object') return undefined;
  const d = value as Partial<DeletionDiagnostic>;
  if (
    typeof d.stage !== 'string' ||
    typeof d.operation !== 'string' ||
    typeof d.category !== 'string' ||
    typeof d.attempt !== 'number' ||
    typeof d.outcome !== 'string'
  )
    return undefined;
  return d as DeletionDiagnostic;
}

export function MpaDeletionDiagnostic({
  diagnostic,
}: {
  diagnostic?: DeletionDiagnostic;
}) {
  const m = useMessages();
  const [copyStatus, setCopyStatus] = useState<'copied' | 'failed'>();
  if (!diagnostic) return null;
  const safe = safeMpaDiagnostic(diagnostic)!;
  const stages: Record<string, string> = {
    checking: m.mpaStageChecking(),
    runtime_sessions: m.mpaDeleteReadSessions(),
    registry: m.mpaDeleteRegistry(),
    references: m.mpaDeleteReferences(),
    managed_resources: m.mpaDeleteInspectResources(),
    worker_template: m.mpaStageWorker(),
    skill_space: m.mpaStageSkills(),
    agentkit_runtime: m.mpaStageRuntime(),
    ownership: m.mpaDeleteReferences(),
  };
  const reasons: Record<string, string> = {
    runtime_binding_conflict: m.mpaDeleteBindingConflict(),
    agent_binding_conflict: m.mpaDeleteBindingConflict(),
    resource_binding_conflict: m.mpaDeleteBindingConflict(),
    ownership_conflict: m.mpaDeleteOwnershipConflict(),
    active_image_update: m.mpaDeleteBusy(),
    active_deployment: m.mpaDeleteBusy(),
  };
  const genericCategories = new Set([
    'unknown',
    'deployment_error',
    'provider_error',
  ]);
  const category =
    /^(AccountFlowLimitExceeded|ServiceFlowLimitExceeded|FlowLimitExceeded|RequestLimitExceeded|Throttling|ThrottlingException|TooManyRequests)([._]|$)/.test(
      safe.errorCode,
    )
      ? 'throttled'
      : diagnostic.category;
  const reason =
    reasons[diagnostic.reason || ''] ||
    (genericCategories.has(category)
      ? m.mpaDeleteFailureUnknown()
      : mpaDiagnosticReasonLabel(m, category));
  const source = /^[A-Za-z0-9_.-]+\.py:[0-9]{1,6}:[A-Za-z0-9_<>]{1,80}$/.test(
    diagnostic.source || '',
  )
    ? diagnostic.source
    : '';
  const host =
    (diagnostic.resourceHost || '').length <= 253 &&
    /^[A-Za-z0-9.-]+$/.test(diagnostic.resourceHost || '')
      ? diagnostic.resourceHost
      : '';
  const fields: Array<[string, string]> = [
    [
      m.mpaFailureStage(),
      stages[diagnostic.stage] || m.mpaDeleteInspectResources(),
    ],
    ...(safe.operation
      ? [[m.mpaFailureOperation(), safe.operation] as [string, string]]
      : []),
    ...(safe.errorCode
      ? [[m.mpaFailureErrorCode(), safe.errorCode] as [string, string]]
      : []),
    ...(safe.exceptionType
      ? [[m.mpaFailureExceptionType(), safe.exceptionType] as [string, string]]
      : []),
    ...(safe.httpStatus
      ? [
          [m.mpaFailureHttpStatus(), String(safe.httpStatus)] as [
            string,
            string,
          ],
        ]
      : []),
    ...(safe.responseType
      ? [[m.mpaFailureResponseType(), safe.responseType] as [string, string]]
      : []),
    ...(safe.requestId
      ? [[m.mpaFailureRequestId(), safe.requestId] as [string, string]]
      : []),
    ...(host ? [[m.mpaDeleteResourceHost(), host] as [string, string]] : []),
    ...(source ? [[m.mpaDeleteErrorSource(), source] as [string, string]] : []),
  ];
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(
        [reason, ...fields.map(([k, v]) => `${k}: ${v}`)].join('\n'),
      );
      setCopyStatus('copied');
    } catch {
      setCopyStatus('failed');
    }
  };
  return (
    <div className="mpa-creation-failure" role="alert">
      <p>{reason}</p>
      <details className="mpa-diagnostic-details">
        <summary>{m.mpaFailureDetails()}</summary>
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
        {copyStatus && (
          <p role="status">
            {copyStatus === 'copied'
              ? m.mpaFailureCopied()
              : m.mpaFailureCopyFailed()}
          </p>
        )}
      </details>
    </div>
  );
}

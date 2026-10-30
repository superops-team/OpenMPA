import type { Diagnostic } from './generated/mygo';
import type { MpaCreationTask } from './lifecycle-models';

const legacyReasons: Record<string, string> = {
  iamPermissionDenied: 'permission',
  iamTemporaryFailure: 'unavailable',
  iamVerificationFailed: 'iamVerification',
  iamTrustConflict: 'iamTrustConflict',
  iamPolicyConflict: 'iamPolicyConflict',
  iamOwnershipConflict: 'ownership',
  iamConfigurationConflict: 'configuration_changed',
  tosBucketUnavailable: 'tosBucketUnavailable',
  runtimeNameCheckFailed: 'nameCheckUnavailable',
  openvikingPermissionDenied: 'permission',
  openvikingQuotaExceeded: 'quota',
  openvikingModelEndpointsInvalid: 'modelEndpoints',
  openvikingModelConfigurationRejected: 'modelConfiguration',
  openvikingConfigurationConflict: 'configuration_changed',
  openvikingCreateUnknown: 'creationUnconfirmed',
  openvikingNotReady: 'metadata_pending',
  openvikingResourceFailed: 'resource_failed',
  openvikingResourceMissing: 'not_found',
  openvikingUnavailable: 'unavailable',
  openvikingInvalidResponse: 'protocol_error',
  configurationChanged: 'configuration_changed',
  timeout: 'timeout',
  interrupted: 'interrupted',
};
const exceptionTypes = new Set([
  'Exception',
  'ApiError',
  'AuthError',
  'NetworkError',
  'HTTPError',
  'HTTPStatusError',
  'RequestException',
  'TimeoutError',
  'Timeout',
  'ConnectTimeout',
  'ReadTimeout',
  'ConnectionError',
  'ConnectError',
  'ReadError',
  'SSLError',
  'JSONDecodeError',
  'ValidationError',
  'ValueError',
  'TypeError',
  'KeyError',
  'RuntimeError',
  'OSError',
]);

export function safeMpaDiagnostic(candidate?: Diagnostic) {
  if (!candidate) return undefined;
  const safeCode = /^[A-Z][A-Za-z0-9]*(?:[._][A-Za-z0-9]+)*$/;
  const code = candidate.errorCode || '';
  const errorCode =
    code.length <= 128 &&
    safeCode.test(code) &&
    !/^(SK_|AK_|ARK_|TOKEN_|BEARER_|SECRET_|PASSWORD_)/i.test(code)
      ? code
      : '';
  const requestId = /^[0-9A-Fa-f-]{16,128}$/.test(candidate.requestId || '')
    ? candidate.requestId
    : '';
  const operation = /^[A-Za-z_]{1,80}$/.test(candidate.operation || '')
    ? candidate.operation
    : '';
  const rawType = candidate.exceptionType || '';
  const parts = rawType.split(' -> ');
  const exceptionType =
    rawType.length <= 256 &&
    parts.length <= 8 &&
    parts.every((part) => exceptionTypes.has(part))
      ? rawType
      : '';
  const httpStatus =
    Number.isInteger(candidate.httpStatus) &&
    candidate.httpStatus! >= 100 &&
    candidate.httpStatus! <= 599
      ? candidate.httpStatus
      : undefined;
  const responseType = [
    'application/json',
    'text/html',
    'text/plain',
    'other',
  ].includes(candidate.responseType || '')
    ? candidate.responseType
    : '';
  return {
    errorCode,
    requestId,
    operation,
    exceptionType,
    httpStatus,
    responseType,
  };
}

export function mpaFailureReason(
  task: MpaCreationTask,
  diagnostic?: Diagnostic,
) {
  const code = safeMpaDiagnostic(diagnostic)?.errorCode || '';
  if (code === 'QuotaExceed.EndpointEachVpc') return 'vpcEndpointQuota';
  if (
    /^(ExceededQuota|QuotaExceed|QuotaExceeded|LimitExceeded)([._]|$)/.test(
      code,
    )
  )
    return 'quota';
  if (/^(AccessDenied|Forbidden|Unauthorized)([._]|$)/.test(code))
    return 'permission';
  if (
    /^(ResourceAlreadyExists|AlreadyExists|DuplicateName)([._]|$)/.test(code) ||
    task.error === 'runtimeNameTaken'
  )
    return 'nameConflict';
  if (/^PostgreSQL\.28[0-9A-Z]{3}$/.test(code)) return 'databaseAuthentication';
  if (code === 'PostgreSQL.42501') return 'databasePermission';
  if (diagnostic?.category === 'resource_failed' && code)
    return 'resourceFailedWithDetails';
  if (task.state === 'failed' && task.error && legacyReasons[task.error])
    return legacyReasons[task.error];
  return diagnostic?.category || 'unknown';
}

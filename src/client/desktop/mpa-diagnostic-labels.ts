import type { Messages } from './generated/locales';

export function mpaDiagnosticReasonLabel(m: Messages, reason: string) {
  const reasons: Record<string, string> = {
    iamTrustConflict: m.mpaFailureReasonIamTrustConflict(),
    iamPolicyConflict: m.mpaFailureReasonIamPolicyConflict(),
    tosBucketUnavailable: m.mpaFailureReasonTosBucketUnavailable(),
    nameCheckUnavailable: m.mpaFailureReasonNameCheckUnavailable(),
    iamVerification: m.mpaFailureReasonIamVerification(),
    modelEndpoints: m.mpaFailureReasonModelEndpoints(),
    modelConfiguration: m.mpaFailureReasonModelConfiguration(),
    creationUnconfirmed: m.mpaFailureReasonCreationUnconfirmed(),
    quota: m.mpaFailureReasonQuota(),
    vpcEndpointQuota: m.mpaFailureReasonVpcEndpointQuota(),
    permission: m.mpaFailureReasonPermission(),
    nameConflict: m.mpaFailureReasonNameConflict(),
    databaseAuthentication: m.mpaFailureReasonDatabaseAuthentication(),
    databasePermission: m.mpaFailureReasonDatabasePermission(),
    timeout: m.mpaFailureReasonTimeout(),
    connection: m.mpaFailureReasonConnection(),
    throttled: m.mpaFailureReasonThrottled(),
    unavailable: m.mpaFailureReasonUnavailable(),
    invalid_request: m.mpaFailureReasonInvalidRequest(),
    not_found: m.mpaFailureReasonNotFound(),
    provider_error: m.mpaFailureReasonProviderError(),
    ownership: m.mpaFailureReasonOwnership(),
    metadata_pending: m.mpaFailureReasonMetadataPending(),
    metadata_missing: m.mpaFailureReasonMetadataMissing(),
    resource_failed: m.mpaFailureReasonResourceFailed(),
    resourceFailedWithDetails: m.mpaFailureReasonResourceFailedWithDetails(),
    configuration_changed: m.mpaFailureReasonConfigurationChanged(),
    deployment_error: m.mpaFailureReasonDeploymentError(),
    unknown: m.mpaFailureReasonUnknown(),
    child_exit: m.mpaFailureReasonChildExit(),
    interrupted: m.mpaFailureReasonInterrupted(),
    protocol_error: m.mpaFailureReasonProtocolError(),
  };
  return reasons[reason] || m.mpaFailureReasonUnknown();
}

import { errorMessage, localized, type MessageText } from './locale';
import { studioFailure } from './studio-services';

export function scheduleFailure(error: unknown): MessageText {
  const code =
    error && typeof error === 'object' && 'code' in error ? error.code : '';
  switch (code) {
    case 'businessIdentityRequired':
    case 'businessJWTRequired':
      return localized((m) => m.scheduleIdentityHint());
    case 'capabilityUnavailable':
      return localized((m) => m.scheduleUnsupported());
    case 'ScheduledTaskVersionConflict':
      return localized((m) => m.scheduleConflictHint());
    case 'ScheduledTaskNotFound':
      return localized((m) => m.scheduleTaskMissing());
    case 'ScheduledTaskQuotaExceeded':
      return localized((m) => m.scheduleQuotaExceeded());
    case 'SchedulerUnavailable':
      return localized((m) => m.scheduleSchedulerUnavailable());
    case 'InvalidDeliveryTarget':
      return localized((m) => m.scheduleSelectTarget());
    case 'IdempotencyConflict':
      return localized((m) => m.conflict());
    case 'InvalidSchedule':
    case 'invalidInput':
      return localized((m) => m.invalidInput());
    default:
      return code
        ? studioFailure(error)
        : errorMessage(
            error,
            localized((m) => m.desktopScheduleOperationFailed()),
          );
  }
}

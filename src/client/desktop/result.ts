export interface SafeFailure {
  code: string;
  message: string;
  httpStatus?: number;
  currentState?: unknown;
}
export type Result<T, E extends SafeFailure = SafeFailure> =
  { ok: true; value: T } | { ok: false; error: E };
export function decodeResult<T, E extends SafeFailure>(result: {
  ok: boolean;
  value?: T | null;
  error?: E | null;
}): Result<T, E> {
  if (result.ok === true && result.value != null && result.error == null)
    return { ok: true, value: result.value };
  if (
    result.ok === false &&
    result.value == null &&
    result.error != null &&
    typeof result.error.code === 'string' &&
    typeof result.error.message === 'string'
  )
    return { ok: false, error: result.error };
  throw new Error('Invalid desktop service result envelope.');
}
export function unwrap<T, E extends SafeFailure>(result: {
  ok: boolean;
  value?: T | null;
  error?: E | null;
}): T {
  const decoded = decodeResult(result);
  if (decoded.ok) return decoded.value;
  const error = Object.assign(new Error(decoded.error.message), decoded.error, {
    status: decoded.error.httpStatus,
  });
  throw error;
}

type RemoteArchitectureLabel = 'MPA' | 'Codex Worker' | 'Runtime';

export interface RemoteArchitecture {
  label: RemoteArchitectureLabel;
  evidence?: string;
}

const codexWorkerPattern = /codex[-_ ]?worker|codexworker/i;
const mpaPattern = /(^|[/_.:-])mpa($|[/_.:-])|mpa[-_ ]?agent/i;

/**
 * The Runtime directory only exposes MPA-tagged Runtimes, while the Worker Tool
 * is the codex-worker association. Use the deployed image, Tool ID and Runtime
 * name as evidence instead of guessing from a label that the API does not
 * expose. When no evidence matches, fall back to the real Runtime identity.
 */
export function remoteArchitecture(input: {
  runtimeName?: string;
  workerToolId?: string;
  runtimeImage?: string;
}): RemoteArchitecture {
  const runtimeEvidence = [input.runtimeImage, input.runtimeName]
    .map((value) => value?.trim())
    .filter((value): value is string => !!value);
  const workerEvidence = input.workerToolId?.trim();
  const runtimeHaystack = runtimeEvidence.join(' ');
  if (codexWorkerPattern.test(runtimeHaystack))
    return {
      label: 'Codex Worker',
      evidence: runtimeEvidence.find((value) => codexWorkerPattern.test(value)),
    };
  if (mpaPattern.test(runtimeHaystack))
    return {
      label: 'MPA',
      evidence: runtimeEvidence.find((value) => mpaPattern.test(value)),
    };
  if (workerEvidence && codexWorkerPattern.test(workerEvidence))
    return { label: 'Codex Worker', evidence: workerEvidence };
  if (workerEvidence && mpaPattern.test(workerEvidence))
    return { label: 'MPA', evidence: workerEvidence };
  return { label: 'Runtime', evidence: runtimeEvidence[0] };
}

export function architectureDisplayLabel(
  architecture: RemoteArchitecture,
  runtimeName?: string,
  runtimeId?: string,
) {
  if (architecture.label !== 'Runtime') return architecture.label;
  return runtimeName || runtimeId || 'Runtime';
}

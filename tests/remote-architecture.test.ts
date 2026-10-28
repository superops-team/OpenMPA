import { expect, it } from 'vitest';
import {
  architectureDisplayLabel,
  remoteArchitecture,
} from '../src/client/desktop/remote-architecture';

it('keeps an MPA Runtime labelled MPA even when it has a codex-worker Tool', () => {
  expect(
    remoteArchitecture({
      runtimeName: 'mpa-agent',
      workerToolId: 't-example-codex-worker',
      runtimeImage: 'registry/mpa-agent:test',
    }).label,
  ).toBe('MPA');
});

it('uses the Runtime image evidence for a codex-worker Runtime', () => {
  expect(
    remoteArchitecture({
      runtimeName: 'codex-worker',
      workerToolId: 't-example-codex-worker',
      runtimeImage: 'registry/mpa-codex-worker:test',
    }).label,
  ).toBe('Codex Worker');
});

it('uses the Runtime image or name evidence for an MPA Runtime', () => {
  expect(
    remoteArchitecture({
      runtimeName: 'mpa-agent',
      runtimeImage: 'registry/mpa-agent:test',
    }).label,
  ).toBe('MPA');
});

it('falls back to the real Runtime identity when no architecture evidence exists', () => {
  const architecture = remoteArchitecture({ runtimeName: 'runtime-one' });
  expect(architecture.label).toBe('Runtime');
  expect(architectureDisplayLabel(architecture, 'runtime-one', 'r-1')).toBe(
    'runtime-one',
  );
  expect(architectureDisplayLabel(architecture, undefined, 'r-1')).toBe('r-1');
});

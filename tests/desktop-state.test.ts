import { describe, expect, it } from 'vitest';
import { decodeResult } from '../src/client/desktop/result';
import {
  projectEvent,
  emptyProjection,
} from '../src/client/desktop/projection';
import { reviewImages } from '../src/client/desktop/image-review';

describe('desktop typed boundary', () => {
  it('rejects malformed success envelopes and preserves safe conflict state', () => {
    expect(() =>
      decodeResult({
        ok: true,
        value: { id: 'x' },
        error: { code: 'bad', message: 'bad' },
      }),
    ).toThrow();
    const error = {
      code: 'conflict',
      message: 'changed',
      httpStatus: 412,
      currentState: { version: '9' },
    };
    expect(decodeResult({ ok: false, error })).toEqual({ ok: false, error });
  });
  it('fences old connections, subscriptions and sessions; deduplicates and replaces partials', () => {
    const scope = { connectionEpoch: 2, subscriptionEpoch: 4, sessionId: 'b' };
    const event = {
      ...scope,
      invocationId: 'run',
      eventId: '1',
      kind: 'message',
      role: 'assistant',
      blockId: 'text',
      text: 'hel',
      partial: true,
    };
    let state = emptyProjection(scope);
    expect(projectEvent(state, { ...event, connectionEpoch: 1 })).toBe(state);
    expect(projectEvent(state, { ...event, sessionId: 'a' })).toBe(state);
    state = projectEvent(state, event);
    expect(projectEvent(state, event)).toBe(state);
    state = projectEvent(state, {
      ...event,
      eventId: '2',
      text: 'hello',
      partial: false,
    });
    expect(state.messages.map((m) => m.content)).toEqual(['hello']);
    expect(
      projectEvent(state, { ...event, eventId: '3', subscriptionEpoch: 3 }),
    ).toBe(state);
  });
  it('reviews exact runtime and tool IDs and rejects incomplete image targets', () => {
    expect(() =>
      reviewImages({
        runtimeId: 'r',
        runtimeVersion: '2',
        runtimeImage: '',
        toolId: 't',
        toolVersion: '3',
        toolImage: 'tool:v4',
      }),
    ).toThrow();
    expect(
      reviewImages({
        runtimeId: 'r',
        runtimeVersion: '2',
        runtimeImage: 'runtime:v3',
        toolId: 't',
        toolVersion: '3',
        toolImage: 'tool:v4',
      }),
    ).toMatchObject({
      runtimeId: 'r',
      toolId: 't',
      expectedRuntimeVersion: '2',
      expectedToolVersion: '3',
    });
  });
  it('retains multiple safe parts sharing one server event ID', () => {
    const scope = { connectionEpoch: 1, subscriptionEpoch: 1, sessionId: 's' };
    const event = {
      ...scope,
      invocationId: 'i',
      eventId: 'server-event',
      kind: 'text',
      role: 'assistant',
      partial: false,
    };
    const first = projectEvent(emptyProjection(scope), {
      ...event,
      blockId: 'part-0',
      text: 'First',
    });
    const next = projectEvent(first, {
      ...event,
      blockId: 'part-1',
      text: 'Second',
    });
    expect(next.messages.map((message) => message.content)).toEqual([
      'First',
      'Second',
    ]);
  });
  it('only completes a matching invocation with a verified terminal event', () => {
    const scope = { connectionEpoch: 1, subscriptionEpoch: 5, sessionId: 's' };
    const state = {
      ...emptyProjection(scope),
      invocationId: 'current',
      running: true,
    };
    const done = {
      ...scope,
      invocationId: 'current',
      eventId: 'd',
      kind: 'done',
    };
    expect(projectEvent(state, done)).toBe(state);
    expect(
      projectEvent(state, { ...done, terminal: true, invocationId: 'old' }),
    ).toBe(state);
    expect(
      projectEvent(state, {
        ...done,
        kind: 'error',
        text: 'Transient status',
        terminal: false,
      }).running,
    ).toBe(true);
    expect(projectEvent(state, { ...done, terminal: true }).running).toBe(
      false,
    );
  });
});

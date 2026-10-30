import { describe, expect, it } from 'vitest';
import {
  calendarTimes,
  monthDays,
  scheduleStatusLabel,
  scheduleText,
} from '../src/client/desktop/schedule-view';
import { messagesIn } from '../src/client/desktop/generated/locales';
import type { ScheduleVM } from '../src/client/desktop/models';
const task = (changes: Partial<ScheduleVM> = {}): ScheduleVM => ({
  id: 'task',
  name: 'Daily task',
  prompt: 'Run',
  agentId: 'main',
  enabled: true,
  version: '1',
  schedule: { type: 'Daily', time: '09:00' },
  timezone: 'Asia/Shanghai',
  ...changes,
});
describe('Runtime task calendar projection', () => {
  it('projects task wall time into the device calendar day', () => {
    const day = new Date(2026, 9, 29);
    const occurrence = calendarTimes(task(), day);
    expect(occurrence).toBeTruthy();
    expect(new Date(occurrence!.at).toISOString()).toBe(
      '2026-10-29T01:00:00.000Z',
    );
    expect(occurrence!.count).toBe(1);
  });
  it('honors weekly and monthly dates, creation time, and paused schedules', () => {
    const day = new Date(2026, 9, 29); // Thursday
    expect(
      calendarTimes(
        task({ schedule: { type: 'Weekly', time: '09:00', weekdays: [4] } }),
        day,
      ),
    ).toBeTruthy();
    expect(
      calendarTimes(
        task({ schedule: { type: 'Weekly', time: '09:00', weekdays: [1] } }),
        day,
      ),
    ).toBeNull();
    expect(
      calendarTimes(
        task({ schedule: { type: 'Monthly', time: '09:00', monthDays: [30] } }),
        day,
      ),
    ).toBeNull();
    expect(
      calendarTimes(task({ createdAt: '2026-10-29T12:00:00Z' }), day),
    ).toBeNull();
    expect(
      calendarTimes(
        task({
          enabled: false,
          schedule: { type: 'Once', runAt: '2026-10-29T01:00:00Z' },
        }),
        day,
      ),
    ).toBeNull();
  });
  it('uses the authoritative next run for complex Cron and never invents an interval anchor', () => {
    const day = new Date(2026, 9, 29);
    const nextRun = '2026-10-29T01:15:00Z';
    expect(
      calendarTimes(
        task({
          schedule: { type: 'Cron', cronExpression: '*/15 9-18 * * MON-FRI' },
          nextRun,
        }),
        day,
      ),
    ).toMatchObject({ at: Date.parse(nextRun), count: 1, nextOnly: true });
    expect(
      calendarTimes(
        task({ schedule: { type: 'Interval', intervalSeconds: 60 } }),
        day,
      ),
    ).toBeNull();
  });
  it('handles DST gaps and repeated times without shifting the schedule', () => {
    expect(
      calendarTimes(
        task({
          timezone: 'America/New_York',
          schedule: { type: 'Daily', time: '02:30' },
        }),
        new Date(2026, 2, 8),
      ),
    ).toBeNull();
    expect(
      calendarTimes(
        task({
          timezone: 'America/New_York',
          schedule: { type: 'Daily', time: '01:30' },
        }),
        new Date(2026, 10, 1),
      )?.count,
    ).toBe(2);
  });
  it('keeps invalid server data from crashing a calendar', () => {
    expect(
      calendarTimes(task({ timezone: 'Invalid/Zone' }), new Date(2026, 9, 29)),
    ).toBeNull();
    expect(monthDays(new Date(2026, 9, 1))).toHaveLength(42);
    expect(monthDays(new Date(2026, 9, 1))[0].getDay()).toBe(1);
  });
  it('localizes schedules and preserves unknown server statuses', () => {
    expect(scheduleText(task(), messagesIn('zh-Hans'))).toContain(
      '每天 · 09:00',
    );
    expect(scheduleStatusLabel('TimedOut', messagesIn('zh-Hans'))).toBe(
      '执行超时',
    );
    expect(scheduleStatusLabel('constructor', messagesIn('en'))).toBe(
      'constructor',
    );
  });
});

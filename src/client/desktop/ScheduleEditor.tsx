import { useEffect, useRef, useState } from 'react';
import { ModalDialog } from '../ui/ModalDialog';
import { Button } from '../ui';
import { localized, useMessages, useMessageState } from './locale';
import { scheduleFailure } from './schedule-failure';
import {
  scheduleTypes,
  scheduleTypeLabel,
  weekdayLabel,
} from './schedule-view';
import type { DesktopServices, ScheduleVM } from './models';

export function ScheduleEditor({
  task,
  services,
  epoch,
  busy,
  error,
  onSave,
  onClose,
}: {
  task: ScheduleVM;
  services: DesktopServices;
  epoch: number;
  busy: boolean;
  error: string;
  onSave(task: ScheduleVM): void;
  onClose(): void;
}) {
  const m = useMessages();
  const [draft, setDraft] = useState(() => structuredClone(task));
  const [validation, setValidation] = useMessageState();
  const [targetError, setTargetError] = useMessageState();
  const [targets, setTargets] = useState<
    Awaited<
      ReturnType<NonNullable<DesktopServices['schedules']>['deliveryTargets']>
    >
  >([]);
  const [bots, setBots] = useState<Array<{ id: string; name: string }>>([]);
  const [loadingTargets, setLoadingTargets] = useState(false);
  const [reload, setReload] = useState(0);
  const [date, setDate] = useState(() => {
    const at = Date.parse(task.schedule.runAt || '');
    if (!Number.isFinite(at)) return '';
    const d = new Date(at);
    return new Date(at - d.getTimezoneOffset() * 60000)
      .toISOString()
      .slice(0, 16);
  });
  const [days, setDays] = useState(task.schedule.monthDays?.join(', ') || '1');
  const composing = useRef(false);
  const formId = 'schedule-editor-form';
  const delivery = draft.delivery || {
    channel: 'Web',
    receiveIdType: 'chat_id',
    bestEffort: true,
  };
  useEffect(() => {
    if (delivery.channel !== 'Feishu') return;
    let alive = true;
    setLoadingTargets(true);
    setTargetError('');
    setTargets([]);
    void (async () => {
      const accounts = services.channels
        ? await services.channels.list(epoch)
        : [];
      if (!alive) return;
      const available = accounts.filter(
        (bot) =>
          bot.type === 'feishu' && bot.enabled && bot.addressable !== false,
      );
      setBots(
        available.map((bot) => ({ id: bot.id, name: bot.name || bot.id })),
      );
      if (!delivery.appId && available.length === 1) {
        setDraft((current) => ({
          ...current,
          delivery: {
            ...delivery,
            appId: available[0].id,
            targetId: undefined,
          },
        }));
        return;
      }
      const value = await services.schedules!.deliveryTargets(
        epoch,
        'Feishu',
        delivery.appId || '',
      );
      if (alive) setTargets(value);
    })()
      .catch((reason) => {
        if (alive) setTargetError(scheduleFailure(reason));
      })
      .finally(() => {
        if (alive) setLoadingTargets(false);
      });
    return () => {
      alive = false;
    };
  }, [services, epoch, delivery.channel, delivery.appId, reload]);
  const changeType = (type: string) => {
    const schedule = {
      type,
      ...(type === 'Once'
        ? {}
        : type === 'Interval'
          ? { intervalSeconds: 3600 }
          : type === 'Cron'
            ? { cronExpression: '0 9 * * *' }
            : {
                time: '09:00',
                ...(type === 'Weekly' ? { weekdays: [1, 2, 3, 4, 5] } : {}),
              }),
    };
    setDraft({ ...draft, schedule });
    setValidation('');
  };
  function save() {
    if (busy || composing.current || loadingTargets) return;
    if (!draft.name.trim() || !draft.prompt.trim()) {
      setValidation(localized((m) => m.scheduleRequired()));
      return;
    }
    try {
      new Intl.DateTimeFormat('en-US', { timeZone: draft.timezone });
      if (!draft.timezone.trim()) throw new Error();
    } catch {
      setValidation(localized((m) => m.scheduleTimezoneInvalid()));
      return;
    }
    const schedule = { ...draft.schedule };
    if (schedule.type === 'Once') {
      const at = Date.parse(date);
      const unchanged =
        !!task.id &&
        date ===
          (() => {
            const d = new Date(task.schedule.runAt || '');
            return Number.isFinite(d.getTime())
              ? new Date(d.getTime() - d.getTimezoneOffset() * 60000)
                  .toISOString()
                  .slice(0, 16)
              : '';
          })();
      if (!Number.isFinite(at) || (!unchanged && at <= Date.now())) {
        setValidation(localized((m) => m.scheduleFutureTime()));
        return;
      }
      schedule.runAt = unchanged
        ? task.schedule.runAt
        : new Date(at).toISOString();
    } else if (schedule.type === 'Interval') {
      if (
        !Number.isInteger(schedule.intervalSeconds) ||
        schedule.intervalSeconds! < 30 ||
        schedule.intervalSeconds! > 31536000
      ) {
        setValidation(localized((m) => m.scheduleIntervalInvalid()));
        return;
      }
    } else if (schedule.type === 'Cron') {
      if (schedule.cronExpression?.trim().split(/\s+/).length !== 5) {
        setValidation(localized((m) => m.scheduleCronInvalid()));
        return;
      }
      schedule.cronExpression = schedule.cronExpression.trim();
    } else {
      if (!/^(?:[01]\d|2[0-3]):[0-5]\d$/.test(schedule.time || '')) {
        setValidation(localized((m) => m.scheduleTimeInvalid()));
        return;
      }
      if (schedule.type === 'Monthly')
        schedule.monthDays = days
          .split(',')
          .map((value) => Number(value.trim()));
      const selected =
        schedule.type === 'Weekly' ? schedule.weekdays : schedule.monthDays;
      if (
        ['Weekly', 'Monthly'].includes(schedule.type) &&
        (!selected?.length ||
          selected.some(
            (day) =>
              !Number.isInteger(day) ||
              day < 1 ||
              day > (schedule.type === 'Weekly' ? 7 : 31),
          ) ||
          new Set(selected).size !== selected.length)
      ) {
        setValidation(localized((m) => m.scheduleDaysInvalid()));
        return;
      }
    }
    if (
      delivery.channel === 'Feishu' &&
      (!delivery.targetId ||
        (bots.length > 0 && !bots.some((bot) => bot.id === delivery.appId)))
    ) {
      setValidation(localized((m) => m.scheduleSelectTarget()));
      return;
    }
    setValidation('');
    onSave({
      ...draft,
      name: draft.name.trim(),
      agentId: draft.agentId.trim(),
      timezone: draft.timezone.trim(),
      schedule,
      delivery:
        delivery.channel === 'Web'
          ? {
              channel: 'Web',
              receiveIdType: 'chat_id',
              bestEffort: delivery.bestEffort,
            }
          : delivery,
    });
  }
  return (
    <ModalDialog
      title={task.id ? m.scheduleEdit() : m.desktopNewScheduledTask()}
      description={m.scheduleEditorHint()}
      className="schedule-dialog"
      closeLabel={m.closeDialog(m.scheduledTasks())}
      onClose={() => {
        if (!busy) onClose();
      }}
      footer={
        <>
          <Button disabled={busy} onClick={onClose}>
            {m.cancel()}
          </Button>
          <Button
            variant="primary"
            type="submit"
            form={formId}
            disabled={busy || loadingTargets}
          >
            {busy ? m.working() : m.desktopSaveSchedule()}
          </Button>
        </>
      }
    >
      <form
        id={formId}
        className="desktop-form schedule-editor"
        onSubmit={(event) => {
          event.preventDefault();
          save();
        }}
        onCompositionStart={() => {
          composing.current = true;
        }}
        onCompositionEnd={() => {
          composing.current = false;
        }}
        onKeyDown={(event) => {
          if (
            event.key === 'Enter' &&
            (composing.current || event.nativeEvent?.isComposing)
          )
            event.preventDefault();
        }}
      >
        {(validation || error) && (
          <p role="alert" className="error-banner">
            {validation || error}
          </p>
        )}
        <fieldset disabled={busy}>
          <label>
            {m.desktopName()}
            <input
              data-autofocus="true"
              required
              maxLength={128}
              value={draft.name}
              autoCapitalize="none"
              autoCorrect="off"
              onChange={(event) =>
                setDraft({ ...draft, name: event.target.value })
              }
            />
          </label>
          <label>
            {m.desktopPrompt()}
            <textarea
              required
              maxLength={32768}
              rows={4}
              value={draft.prompt}
              onChange={(event) =>
                setDraft({ ...draft, prompt: event.target.value })
              }
            />
          </label>
          <div className="schedule-form-grid">
            <label>
              {m.desktopScheduleType()}
              <select
                aria-label={m.desktopScheduleType()}
                value={draft.schedule.type}
                onChange={(event) => changeType(event.target.value)}
              >
                {scheduleTypes.map((type) => (
                  <option key={type} value={type}>
                    {scheduleTypeLabel(type, m)}
                  </option>
                ))}
              </select>
            </label>
            <label>
              {m.desktopTimeZone()}
              <input
                required
                value={draft.timezone}
                placeholder="Asia/Shanghai"
                onChange={(event) =>
                  setDraft({ ...draft, timezone: event.target.value })
                }
              />
            </label>
          </div>
          {draft.schedule.type === 'Once' && (
            <label>
              {m.scheduleRunAtLocal()}
              <input
                required
                type="datetime-local"
                value={date}
                onChange={(event) => setDate(event.target.value)}
              />
            </label>
          )}
          {draft.schedule.type === 'Interval' && (
            <label>
              {m.desktopIntervalSeconds()}
              <input
                required
                type="number"
                min={30}
                max={31536000}
                step={1}
                value={draft.schedule.intervalSeconds ?? ''}
                onChange={(event) =>
                  setDraft({
                    ...draft,
                    schedule: {
                      ...draft.schedule,
                      intervalSeconds: Number(event.target.value),
                    },
                  })
                }
              />
            </label>
          )}
          {draft.schedule.type === 'Cron' && (
            <label>
              {m.desktopCronExpression()}
              <input
                required
                maxLength={128}
                placeholder="0 9 * * *"
                value={draft.schedule.cronExpression || ''}
                onChange={(event) =>
                  setDraft({
                    ...draft,
                    schedule: {
                      ...draft.schedule,
                      cronExpression: event.target.value,
                    },
                  })
                }
              />
              <small>{m.scheduleCronInvalid()}</small>
            </label>
          )}
          {['Daily', 'Weekly', 'Monthly'].includes(draft.schedule.type) && (
            <label>
              {m.scheduleTime()}
              <input
                required
                type="time"
                value={draft.schedule.time || ''}
                onChange={(event) =>
                  setDraft({
                    ...draft,
                    schedule: { ...draft.schedule, time: event.target.value },
                  })
                }
              />
            </label>
          )}
          {draft.schedule.type === 'Weekly' && (
            <fieldset className="schedule-weekdays">
              <legend>{m.desktopWeekdays1MondayThrough7Sunday()}</legend>
              {[1, 2, 3, 4, 5, 6, 7].map((day) => (
                <label key={day}>
                  <input
                    type="checkbox"
                    checked={draft.schedule.weekdays?.includes(day) || false}
                    onChange={(event) =>
                      setDraft({
                        ...draft,
                        schedule: {
                          ...draft.schedule,
                          weekdays: event.target.checked
                            ? [...(draft.schedule.weekdays || []), day].sort()
                            : draft.schedule.weekdays?.filter(
                                (item) => item !== day,
                              ),
                        },
                      })
                    }
                  />
                  {weekdayLabel(day, m)}
                </label>
              ))}
            </fieldset>
          )}
          {draft.schedule.type === 'Monthly' && (
            <label>
              {m.desktopMonthDays1Through31()}
              <input
                required
                value={days}
                onChange={(event) => setDays(event.target.value)}
              />
            </label>
          )}
          <label>
            {m.desktopDeliveryChannel()}
            <select
              aria-label={m.desktopDeliveryChannel()}
              value={delivery.channel}
              onChange={(event) => {
                setTargets([]);
                setDraft({
                  ...draft,
                  delivery: {
                    channel: event.target.value,
                    receiveIdType: 'chat_id',
                    bestEffort: true,
                  },
                });
              }}
            >
              <option value="Web">{m.desktopWeb()}</option>
              <option value="Feishu">{m.desktopFeishu()}</option>
            </select>
          </label>
          {delivery.channel === 'Feishu' && (
            <div className="schedule-delivery">
              {targetError && (
                <p role="alert" className="error-banner">
                  {targetError}
                </p>
              )}
              {bots.length > 0 && (
                <label>
                  {m.scheduleBot()}
                  <select
                    required
                    aria-label={m.scheduleBot()}
                    value={delivery.appId || ''}
                    onChange={(event) =>
                      setDraft({
                        ...draft,
                        delivery: {
                          ...delivery,
                          appId: event.target.value,
                          targetId: undefined,
                        },
                      })
                    }
                  >
                    <option value="">{m.scheduleSelectBot()}</option>
                    {bots.map((bot) => (
                      <option key={bot.id} value={bot.id}>
                        {bot.name}
                      </option>
                    ))}
                  </select>
                </label>
              )}
              <Button
                disabled={loadingTargets}
                onClick={() => setReload((value) => value + 1)}
              >
                {m.desktopReadPermittedDeliveryTargets()}
              </Button>
              <label>
                {m.desktopDeliveryTarget()}
                <select
                  required
                  disabled={loadingTargets}
                  aria-label={m.desktopDeliveryTarget()}
                  value={delivery.targetId || ''}
                  onChange={(event) =>
                    setDraft({
                      ...draft,
                      delivery: { ...delivery, targetId: event.target.value },
                    })
                  }
                >
                  <option value="">
                    {loadingTargets
                      ? m.working()
                      : m.desktopChooseAPermittedGroup()}
                  </option>
                  {delivery.targetId &&
                    !targets.some(
                      (target) => target.targetId === delivery.targetId,
                    ) && (
                      <option value={delivery.targetId}>
                        {delivery.targetId}
                      </option>
                    )}
                  {targets
                    .filter(
                      (target) =>
                        !delivery.appId ||
                        !target.appId ||
                        target.appId === delivery.appId,
                    )
                    .map((target) => (
                      <option
                        key={`${target.appId}:${target.targetId}`}
                        value={target.targetId}
                      >
                        {target.name || target.targetId}
                      </option>
                    ))}
                </select>
              </label>
            </div>
          )}
          <label className="schedule-check">
            <input
              type="checkbox"
              checked={draft.enabled}
              onChange={(event) =>
                setDraft({ ...draft, enabled: event.target.checked })
              }
            />
            {m.scheduleEnabledField()}
          </label>
          <details>
            <summary>{m.channelAdvancedSettings()}</summary>
            <label>
              {m.desktopAgentIDOptional()}
              <input
                maxLength={256}
                value={draft.agentId}
                onChange={(event) =>
                  setDraft({ ...draft, agentId: event.target.value })
                }
              />
              <small>{m.scheduleAgentHint()}</small>
            </label>
          </details>
        </fieldset>
      </form>
    </ModalDialog>
  );
}

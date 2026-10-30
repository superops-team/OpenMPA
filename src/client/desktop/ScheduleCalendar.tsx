import { useMemo, useState } from 'react';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import { Button, IconButton } from '../ui';
import { useMessages } from './locale';
import { calendarTimes, monthDays, weekdayLabel } from './schedule-view';
import type { ScheduleVM } from './models';
import { currentLocale } from 'mygo-runtime/i18n';

export function ScheduleCalendar({
  tasks,
  onOpen,
}: {
  tasks: ScheduleVM[];
  onOpen(task: ScheduleVM): void;
}) {
  const m = useMessages();
  const [month, setMonth] = useState(() => new Date());
  const calendar = useMemo(
    () =>
      monthDays(month).map((day) => ({
        day,
        tasks: tasks
          .flatMap((task) => {
            const entry = calendarTimes(task, day);
            return entry ? [{ task, ...entry }] : [];
          })
          .sort((a, b) => a.at - b.at),
      })),
    [tasks, month],
  );
  const today = new Date().toDateString();
  const renderTask = ({
    task,
    at,
    count,
    nextOnly,
  }: (typeof calendar)[number]['tasks'][number]) => (
    <button
      key={task.id}
      className="schedule-calendar-task"
      onClick={() => onOpen(task)}
      title={task.name}
    >
      <span>
        {new Intl.DateTimeFormat(currentLocale(), {
          hour: '2-digit',
          minute: '2-digit',
          hourCycle: 'h23',
        }).format(at)}
      </span>{' '}
      <strong>{task.name}</strong>
      {count > 1 && <small>{m.scheduleOccurrences(String(count))}</small>}
      {nextOnly && <small>{m.scheduleNextOnly()}</small>}
    </button>
  );
  return (
    <section
      className="schedule-calendar"
      aria-label={m.scheduleCalendarView()}
    >
      <div className="schedule-calendar-heading">
        <h2>
          {new Intl.DateTimeFormat(currentLocale(), {
            year: 'numeric',
            month: 'long',
          }).format(month)}
        </h2>
        <div className="schedule-actions">
          <IconButton
            label={m.scheduleMonthPrevious()}
            onClick={() =>
              setMonth(new Date(month.getFullYear(), month.getMonth() - 1, 1))
            }
          >
            <ChevronLeft size={16} />
          </IconButton>
          <Button onClick={() => setMonth(new Date())}>
            {m.scheduleToday()}
          </Button>
          <IconButton
            label={m.scheduleMonthNext()}
            onClick={() =>
              setMonth(new Date(month.getFullYear(), month.getMonth() + 1, 1))
            }
          >
            <ChevronRight size={16} />
          </IconButton>
        </div>
      </div>
      <p className="schedule-hint">{m.scheduleCalendarHint()}</p>
      <div className="schedule-calendar-scroll">
        <div className="schedule-calendar-grid">
          {[1, 2, 3, 4, 5, 6, 7].map((day) => (
            <div className="schedule-calendar-weekday" key={day}>
              {weekdayLabel(day, m)}
            </div>
          ))}
          {calendar.map(({ day, tasks }) => (
            <div
              className="schedule-calendar-day"
              key={day.toISOString()}
              data-current-month={day.getMonth() === month.getMonth()}
            >
              <time
                dateTime={`${day.getFullYear()}-${String(day.getMonth() + 1).padStart(2, '0')}-${String(day.getDate()).padStart(2, '0')}`}
                aria-current={day.toDateString() === today ? 'date' : undefined}
              >
                {day.getDate()}
              </time>
              {tasks.slice(0, 3).map(renderTask)}
              {tasks.length > 3 && (
                <details className="schedule-calendar-more">
                  <summary>
                    {m.scheduleMoreTasks(String(tasks.length - 3))}
                  </summary>
                  {tasks.slice(3).map(renderTask)}
                </details>
              )}
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}

import type { Messages } from './generated/locales';

export type DesktopView =
  | 'chat'
  | 'tasks'
  | 'channels'
  | 'settings'
  | 'knowledge'
  | 'resources'
  | 'collaboration'
  | 'runtimes';

export function desktopPageTitle(m: Messages, view: DesktopView) {
  switch (view) {
    case 'runtimes':
      return m.manageRuntimes();
    case 'resources':
      return m.resources();
    case 'collaboration':
      return m.collaboration();
    case 'settings':
      return m.settings();
    case 'channels':
      return m.channels();
    case 'knowledge':
      return m.knowledge();
    case 'tasks':
      return m.scheduledTasks();
    default:
      return m.conversations();
  }
}

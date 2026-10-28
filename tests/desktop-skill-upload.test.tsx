import { act, create, type ReactTestRenderer } from 'react-test-renderer';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { SkillUploadButton } from '../src/client/desktop/SkillUpload';
import { messagesIn } from '../src/client/desktop/generated/locales';
import type {
  ResourcesServices,
  SkillUploadVM,
} from '../src/client/desktop/studio-models';
let renderer: ReactTestRenderer | undefined;
const m = messagesIn('en');
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  vi.stubGlobal('mygo', { locale: 'en' });
  vi.stubGlobal(
    'FileReader',
    class {
      result = 'data:application/zip;base64,cGFja2FnZQ==';
      onload?: () => void;
      readAsDataURL() {
        this.onload?.();
      }
    },
  );
});
afterEach(async () => {
  if (renderer) await act(async () => renderer!.unmount());
  renderer = undefined;
  vi.useRealTimers();
  vi.unstubAllGlobals();
});
const button = (label: string) =>
  renderer!.root
    .findAllByType('button')
    .find((b) => b.children.some((c) => c === label))!;
const view: SkillUploadVM = {
  operationId: 'op',
  spaceId: 'bound-space',
  name: 'review',
  skillId: 'skill-new',
  version: '',
  status: 'processing',
};
async function mount(
  overrides: Partial<ResourcesServices> = {},
  onUploaded = vi.fn(async () => {}),
) {
  const services = {
    uploadSkill: vi.fn(async () => view),
    readSkillUpload: vi.fn(async () => ({
      ...view,
      version: '1',
      status: 'published',
    })),
    ...overrides,
  } as unknown as ResourcesServices;
  await act(async () => {
    renderer = create(
      <SkillUploadButton
        services={services}
        epoch={5}
        space={{ id: 'bound-space', name: 'Team skills' }}
        onUploaded={onUploaded}
      />,
    );
  });
  await act(async () => button(m.uploadBoundSkill()).props.onClick());
  return { services, onUploaded };
}
it('uploads the chosen package only to the bound space and refreshes after publication', async () => {
  vi.useFakeTimers();
  const { services, onUploaded } = await mount();
  await act(async () =>
    renderer!.root.findByProps({ type: 'file' }).props.onChange({
      target: { files: [{ name: 'review.zip', size: 100 }] },
    }),
  );
  await act(async () => button(m.skillUploadAndPublish()).props.onClick());
  expect(services.uploadSkill).toHaveBeenCalledWith(
    5,
    'bound-space',
    expect.any(String),
    'review.zip',
    'cGFja2FnZQ==',
  );
  expect(onUploaded).not.toHaveBeenCalled();
  await act(async () => {
    await vi.advanceTimersByTimeAsync(2500);
  });
  expect(onUploaded).toHaveBeenCalledExactlyOnceWith('bound-space');
  expect(JSON.stringify(renderer!.toJSON())).toContain(m.skillPublished());
  await act(async () => {
    await vi.advanceTimersByTimeAsync(5000);
  });
  expect(services.readSkillUpload).toHaveBeenCalledOnce();
});
it('rejects an oversized file before reading or sending it', async () => {
  const { services } = await mount();
  await act(async () =>
    renderer!.root.findByProps({ type: 'file' }).props.onChange({
      target: { files: [{ name: 'huge.zip', size: 21 * 1024 * 1024 }] },
    }),
  );
  expect(button(m.skillUploadAndPublish()).props.disabled).toBe(true);
  expect(services.uploadSkill).not.toHaveBeenCalled();
  expect(JSON.stringify(renderer!.toJSON())).toContain(
    m.skillPackageTooLarge(),
  );
});
it('keeps an uncertain outcome read-only and does not resubmit on status checks', async () => {
  const uploadSkill = vi.fn(async () => {
    throw { code: 'outcomeUnknown' };
  });
  const readSkillUpload = vi.fn(async () => ({ ...view, status: 'unknown' }));
  await mount({ uploadSkill, readSkillUpload });
  await act(async () =>
    renderer!.root.findByProps({ type: 'file' }).props.onChange({
      target: { files: [{ name: 'review.zip', size: 100 }] },
    }),
  );
  await act(async () => button(m.skillUploadAndPublish()).props.onClick());
  expect(renderer!.root.findByProps({ type: 'file' }).props.disabled).toBe(
    true,
  );
  await act(async () => button(m.skillReadUploadStatus()).props.onClick());
  expect(uploadSkill).toHaveBeenCalledOnce();
  expect(readSkillUpload).toHaveBeenCalledOnce();
});

import { useState } from 'react';
import {
  Edit3,
  RefreshCw,
  Save,
  FileText,
  Cpu,
  BookOpen,
  Server,
} from 'lucide-react';
import { AssistantMarkdown } from './AssistantMarkdown';
import type { ConnectionVM } from './models';
import type {
  MainProfileOutcome,
  MainProfileReadVM,
  MainProfileVM,
  ResourcesServices,
} from './studio-models';
import { localized, LocalizedError, useMessages } from './locale';
import { useResourceEditor } from './use-resource-editor';
import { mainProfileIssue, mergeMainProfile } from './main-profile-form';
import { MainSkillFields, MainMCPFields } from './MainProfileFields';

export function MainProfileEditor({
  services,
  connection,
  hidden,
}: {
  services: ResourcesServices;
  connection: ConnectionVM;
  hidden: boolean;
}) {
  const m = useMessages();
  const [canCreate, setCanCreate] = useState(false);
  const configured = (outcome: MainProfileOutcome): MainProfileReadVM => {
    if (
      outcome.status === 'configured' &&
      outcome.value?.record &&
      outcome.value.snapshotId
    )
      return outcome.value;
    if (
      outcome.status === 'unsupportedOrUnconfigured' ||
      outcome.status === 'unsupported' ||
      outcome.status === 'unconfigured'
    )
      throw new LocalizedError(
        localized((messages) =>
          outcome.status === 'unsupported'
            ? messages.mainProfileUnsupported()
            : outcome.status === 'unconfigured'
              ? messages.mainProfileUnconfigured()
              : messages.mainProfileUnsupportedOrUnconfigured(),
        ),
      );
    throw (
      outcome.error ??
      new LocalizedError(
        localized((messages) => messages.mainProfileUnavailable()),
      )
    );
  };
  const editor = useResourceEditor<MainProfileReadVM, MainProfileVM>({
    hidden,
    savedMessage: localized((messages) => messages.mainProfileSaved()),
    read: async () => {
      const outcome = await services.readMainProfile(connection.epoch);
      setCanCreate(
        outcome.status === 'unconfigured' ||
          outcome.status === 'unsupportedOrUnconfigured',
      );
      return configured(outcome);
    },
    write: async (record, draft) => {
      if (record.status === 'unconfigured' && services.createMainProfile) {
        const result = configured(
          await services.createMainProfile(connection.epoch, draft),
        );
        setCanCreate(false);
        return result;
      }
      if (!services.saveMainProfile)
        throw new LocalizedError(
          localized((messages) => messages.capabilityUnavailable()),
        );
      return configured(
        await services.saveMainProfile(
          connection.epoch,
          record.snapshotId!,
          draft,
        ),
      );
    },
    value: (record) =>
      record.record?.profile ?? { agentsMd: '', model: null, skills: [] },
    merge: mergeMainProfile,
  });
  const profile = editor.editing
    ? editor.draft
    : editor.record?.record?.profile;
  const issue =
    profile && editor.editing ? mainProfileIssue(profile) : undefined;
  const update = (patch: Partial<MainProfileVM>) => {
    if (profile) editor.update({ ...profile, ...patch });
  };
  return (
    <section
      className="resource-document main-profile-document"
      hidden={hidden}
      style={hidden ? { display: 'none' } : undefined}
      data-testid="resources-mainProfile"
    >
      <header className="resource-editor-toolbar">
        <div>
          <h2>
            {editor.record?.status === 'unconfigured'
              ? m.mainCreateProfile()
              : m.mainProfile()}
          </h2>
          <p>{m.mainProfileScope()}</p>
        </div>
        <div className="resource-editor-actions">
          <button
            type="button"
            className="resource-button"
            disabled={editor.busy}
            aria-label={m.reloadMainProfile()}
            title={m.reloadMainProfile()}
            onClick={() => void editor.refresh()}
          >
            <RefreshCw size={16} aria-hidden="true" />
          </button>
          {editor.editing ? (
            <>
              <button
                type="button"
                className="resource-button"
                disabled={editor.busy}
                onClick={editor.cancel}
              >
                {m.cancel()}
              </button>
              <button
                type="button"
                className="resource-button is-primary"
                disabled={
                  editor.busy || !editor.fresh || !editor.dirty || !!issue
                }
                onClick={() => void editor.save()}
              >
                <Save size={16} aria-hidden="true" />
                {m.saveMainProfile()}
              </button>
            </>
          ) : (
            <button
              type="button"
              className="resource-button"
              disabled={
                editor.busy || !editor.record || !services.saveMainProfile
              }
              onClick={editor.begin}
            >
              <Edit3 size={16} aria-hidden="true" />
              {m.profileEdit()}
            </button>
          )}
        </div>
      </header>
      {editor.error && (
        <p role="alert" className="resource-editor-error">
          {editor.error}
        </p>
      )}
      {editor.notice && (
        <p role="status" className="resource-editor-note">
          {editor.notice}
        </p>
      )}
      {editor.busy && !profile && <p role="status">{m.working()}</p>}
      {!editor.busy && !profile && canCreate && services.createMainProfile && (
        <button
          type="button"
          className="resource-button"
          onClick={() =>
            editor.initialize({
              status: 'unconfigured',
              connectionEpoch: connection.epoch,
            })
          }
        >
          {m.mainCreateProfile()}
        </button>
      )}
      {!editor.busy && !profile && !editor.error && (
        <p className="resource-editor-note">{m.profileReadFailed()}</p>
      )}
      {profile && (
        <>
          <div className="profile-meta">
            <span>
              {m.loadedRevision()} {editor.record?.record?.revision ?? '—'}
            </span>
            <span>{m.mainNextTurn()}</span>
            {editor.dirty && <span>{m.profileUnsaved()}</span>}
          </div>
          {editor.editing && !editor.fresh && (
            <p role="status" className="resource-editor-note">
              {m.readInstructionsBeforeSaving()}
            </p>
          )}
          {issue && (
            <p role="alert" className="resource-editor-error">
              {issue === 'credentials'
                ? m.mainCredentialsRequired()
                : m.profileInvalidFields()}
            </p>
          )}
          <section
            className="profile-section"
            aria-labelledby="main-instructions-heading"
          >
            <h3 id="main-instructions-heading">
              <FileText size={17} aria-hidden="true" />
              {m.mainInstructionsHeading()}
            </h3>
            {editor.editing ? (
              <textarea
                aria-label={m.mainProfileInstructions()}
                data-testid="resources-mainProfile-draft"
                className="resource-document-editor is-main"
                spellCheck={false}
                maxLength={40000}
                value={profile.agentsMd}
                onChange={(e) => update({ agentsMd: e.target.value })}
              />
            ) : (
              <div className="resource-document-body">
                {profile.agentsMd ? (
                  <AssistantMarkdown text={profile.agentsMd} />
                ) : (
                  <p className="resource-editor-note">
                    {m.profileNoInstructions()}
                  </p>
                )}
              </div>
            )}
          </section>
          <section
            className="profile-section"
            aria-labelledby="main-model-heading"
          >
            <h3 id="main-model-heading">
              <Cpu size={17} aria-hidden="true" />
              {m.model()}
            </h3>
            {editor.editing ? (
              <div className="profile-model-fields">
                <label>
                  {m.mainModelMode()}
                  <select
                    value={profile.model === null ? 'inherit' : 'fixed'}
                    onChange={(e) =>
                      update({
                        model: e.target.value === 'inherit' ? null : { id: '' },
                      })
                    }
                  >
                    <option value="inherit">{m.mainModelInherited()}</option>
                    <option value="fixed">{m.mainModelFixed()}</option>
                  </select>
                </label>
                {profile.model !== null && (
                  <label>
                    {m.mainModelId()}
                    <input
                      value={profile.model.id}
                      maxLength={512}
                      onChange={(e) =>
                        update({ model: { id: e.target.value } })
                      }
                    />
                  </label>
                )}
              </div>
            ) : (
              <p className="profile-model-value">
                {profile.model?.id || m.mainModelInherited()}
              </p>
            )}
            <p className="resource-editor-note">{m.mainModelHint()}</p>
          </section>
          <section
            className="profile-section"
            aria-labelledby="main-skills-heading"
          >
            <h3 id="main-skills-heading">
              <BookOpen size={17} aria-hidden="true" />
              {m.skills()}{' '}
              <span className="profile-count">{profile.skills.length}</span>
            </h3>
            <MainSkillFields
              skills={profile.skills}
              editing={editor.editing}
              onChange={(skills) => update({ skills })}
            />
          </section>
          <section
            className="profile-section"
            aria-labelledby="main-mcp-heading"
          >
            <h3 id="main-mcp-heading">
              <Server size={17} aria-hidden="true" />
              MCP{' '}
              <span className="profile-count">
                {(profile.mcpSources?.length ?? 0) +
                  Object.keys(profile.mcpServers ?? {}).length}
              </span>
            </h3>
            <MainMCPFields
              profile={profile}
              editing={editor.editing}
              onChange={editor.update}
            />
          </section>
          {editor.editing && (
            <footer className="profile-save-footer">
              <span className="resource-editor-note">
                {editor.dirty ? m.profileUnsaved() : ''}
              </span>
              <div className="resource-editor-actions">
                <button
                  type="button"
                  className="resource-button"
                  disabled={editor.busy}
                  onClick={editor.cancel}
                >
                  {m.cancel()}
                </button>
                <button
                  type="button"
                  className="resource-button is-primary"
                  disabled={
                    editor.busy || !editor.fresh || !editor.dirty || !!issue
                  }
                  onClick={() => void editor.save()}
                >
                  <Save size={16} aria-hidden="true" />
                  {m.saveMainProfile()}
                </button>
              </div>
            </footer>
          )}
        </>
      )}
    </section>
  );
}

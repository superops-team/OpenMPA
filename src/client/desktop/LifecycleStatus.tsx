import { useMessages } from './locale';

export function LifecycleStatus({ state }: { state: string }) {
  const m = useMessages();
  const label: Record<string, string> = {
    running: m.mpaRunning(),
    cancelling: m.mpaCancelling(),
    succeeded: m.mpaSucceeded(),
    failed: m.mpaFailed(),
    cancelled: m.mpaCancelled(),
    unknown: m.mpaUnknown(),
  };
  return (
    <span className="mpa-status" data-state={state}>
      {label[state] || m.mpaUnknown()}
    </span>
  );
}
export function LifecycleStage({
  stage,
  deleting = false,
}: {
  stage: string;
  deleting?: boolean;
}) {
  const m = useMessages();
  const label: Record<string, string> = {
    checking: m.mpaStageChecking(),
    complete: m.mpaStageComplete(),
    runtime_sessions: m.mpaStageSessions(),
    managed_resources: m.mpaStageResources(),
    agentkit_runtime: m.mpaStageRuntime(),
    worker: deleting ? m.mpaStageWorker() : m.mpaStageCreateWorker(),
    worker_template: m.mpaStageWorker(),
    skill_space: m.mpaStageSkills(),
    skills: deleting ? m.mpaStageSkills() : m.mpaStageCreateSkills(),
    iam_role: m.mpaStageIam(),
    admin_workspace: m.mpaStageAdminWorkspace(),
    business_workspace: m.mpaStageBusinessWorkspace(),
    admin_database: m.mpaStageDatabase(),
    database: m.mpaStageDatabase(),
    network: m.mpaStageNetwork(),
    gateway: m.mpaStageGateway(),
    openviking: m.mpaStageOpenViking(),
    openviking_key: m.mpaStageOpenViking(),
    deploying: m.mpaStageDeploying(),
    verifying: m.mpaStageVerifying(),
  };
  return <span>{label[stage] || m.mpaStagePreparing()}</span>;
}

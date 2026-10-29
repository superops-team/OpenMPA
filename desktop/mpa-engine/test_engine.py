import asyncio
from contextlib import asynccontextmanager
from copy import deepcopy
from types import SimpleNamespace
from unittest.mock import AsyncMock
from uuid import uuid4

import pytest
from frontend.server.mpa.runtime_client import MpaRuntimeSessionSummary

from engine import Engine, LifecycleError


@pytest.fixture
def harness(tmp_path):
    record = {
        "RuntimeId": "r-one", "Name": "My MPA", "Status": "Ready", "CurrentVersionNumber": 1,
        "Tags": [{"Key": "veadk:agent-type", "Value": "mpa"}],
        "Envs": [{"Key": "MPA_AGENT_ID", "Value": "mi-one"}],
        "Endpoint": "https://runtime.invalid",
        "AuthorizerConfiguration": {"KeyAuth": {"ApiKey": "DO_NOT_EXPOSE"}},
    }
    cloud = SimpleNamespace(account_id=AsyncMock(return_value="account"), get=AsyncMock(side_effect=lambda _: deepcopy(record)), list_runtimes=AsyncMock(side_effect=lambda: [deepcopy(record)]), call=AsyncMock())
    runtime = SimpleNamespace(list_sessions=AsyncMock(return_value=[MpaRuntimeSessionSummary(sessionId="session-1", status="idle")]), delete_session=AsyncMock())
    cleanup = SimpleNamespace(preview=AsyncMock(return_value={"cleanupPlan": [{"stage": "worker_template", "resourceId": "t-one", "count": 1}], "retainedResources": [{"stage": "skill_space", "resourceId": "shared", "reason": "shared"}]}), delete_resources=AsyncMock(), mark_runtime_deleted=AsyncMock())

    @asynccontextmanager
    async def scope(**_kwargs):
        yield cleanup

    creations = SimpleNamespace(list=lambda *_a, **_k: [], close=AsyncMock())
    engine = Engine("account", "owner", tmp_path, cloud_factory=lambda _: cloud, runtime_client=runtime, cleanup=scope, creations=creations)
    return SimpleNamespace(engine=engine, cloud=cloud, runtime=runtime, cleanup=cleanup, record=record, target={"runtimeId": "r-one", "region": "cn-beijing"})


@pytest.mark.asyncio
async def test_preview_is_read_only_and_excludes_secrets(harness):
    h = harness
    preview = await h.engine.dispatch("previewDelete", h.target)
    assert preview["sessionCounts"] == {"total": 1, "active": 0, "idle": 1}
    assert preview["canDelete"]
    assert preview["retainedResources"][0]["resourceId"] == "shared"
    assert "DO_NOT_EXPOSE" not in str(preview)
    h.runtime.delete_session.assert_not_called()
    h.cleanup.delete_resources.assert_not_called()
    h.cloud.call.assert_not_called()
    await h.engine.close()


@pytest.mark.asyncio
@pytest.mark.parametrize("status", ["running", "paused", "pausing", "resuming", "queued"])
async def test_active_sessions_block_deletion(harness, status):
    h = harness
    h.runtime.list_sessions.return_value = [MpaRuntimeSessionSummary(sessionId="s", status=status)]
    preview = await h.engine.preview(h.target)
    assert not preview["canDelete"]
    with pytest.raises(LifecycleError) as failure:
        await h.engine.dispatch("delete", {**h.target, "fingerprint": preview["fingerprint"], "operationId": str(uuid4()), "confirm": True})
    assert failure.value.code == "staleDeletePreview"
    h.cloud.call.assert_not_called()
    await h.engine.close()


@pytest.mark.asyncio
async def test_confirm_revalidates_fingerprint_and_deletes_once(harness):
    h = harness
    preview = await h.engine.preview(h.target)
    request = {**h.target, "fingerprint": preview["fingerprint"], "operationId": str(uuid4()), "confirm": True}
    value = await h.engine.dispatch("delete", request)
    assert value["state"] == "running"
    task = h.engine.deleting[request["operationId"]]
    await task
    repeated = await h.engine.dispatch("delete", request)
    assert repeated["state"] == "succeeded"
    h.runtime.delete_session.assert_awaited_once()
    h.cleanup.delete_resources.assert_awaited_once()
    h.cloud.call.assert_awaited_once()
    assert h.cloud.call.call_args.args[0] == "delete_runtime"
    assert h.cloud.call.call_args.args[1].runtime_id == "r-one"
    h.cleanup.mark_runtime_deleted.assert_awaited_once()
    with pytest.raises(LifecycleError) as failure:
        await h.engine.dispatch("delete", {**request, "runtimeId": "r-other"})
    assert failure.value.code == "conflict"
    await h.engine.close()


@pytest.mark.asyncio
async def test_changed_preview_and_account_never_delete(harness):
    h = harness
    preview = await h.engine.preview(h.target)
    request = {**h.target, "fingerprint": preview["fingerprint"], "operationId": str(uuid4()), "confirm": True}
    h.record["CurrentVersionNumber"] = 2
    with pytest.raises(LifecycleError) as failure:
        await h.engine.dispatch("delete", request)
    assert failure.value.code == "staleDeletePreview"
    h.cloud.account_id.return_value = "another-account"
    with pytest.raises(LifecycleError) as failure:
        await h.engine.preview(h.target)
    assert failure.value.code == "scopeMismatch"
    h.cloud.call.assert_not_called()
    await h.engine.close()


@pytest.mark.asyncio
async def test_lost_delete_receipt_is_retained_without_resubmission(harness):
    h = harness
    h.cloud.call.side_effect = TimeoutError("private upstream details")
    preview = await h.engine.preview(h.target)
    request = {**h.target, "fingerprint": preview["fingerprint"], "operationId": str(uuid4()), "confirm": True}
    await h.engine.dispatch("delete", request)
    await h.engine.deleting[request["operationId"]]
    value = await h.engine.dispatch("delete", request)
    assert value["state"] == "unknown"
    assert "private" not in str(value)
    h.cloud.call.assert_awaited_once()
    await h.engine.close()


@pytest.mark.asyncio
async def test_runtime_becomes_busy_after_confirmation(harness):
    h = harness
    preview = await h.engine.preview(h.target)
    request = {**h.target, "fingerprint": preview["fingerprint"], "operationId": str(uuid4()), "confirm": True}
    await h.engine.dispatch("delete", request)
    h.runtime.list_sessions.return_value = [MpaRuntimeSessionSummary(sessionId="s", status="running")]
    await h.engine.deleting[request["operationId"]]
    value = await h.engine.read_delete({"operationId": request["operationId"], "region": "cn-beijing"})
    assert value["state"] == "failed"
    h.runtime.delete_session.assert_not_called()
    h.cloud.call.assert_not_called()
    await h.engine.close()


@pytest.mark.asyncio
async def test_ambiguous_agent_binding_and_untrusted_endpoint_are_rejected(harness):
    h = harness
    other = deepcopy(h.record)
    other["RuntimeId"] = "r-other"
    h.cloud.list_runtimes.side_effect = None
    h.cloud.list_runtimes.return_value = [h.record, other]
    with pytest.raises(LifecycleError) as failure:
        await h.engine.preview(h.target)
    assert failure.value.code == "ownershipConflict"
    h.record["Endpoint"] = "https://user:password@runtime.invalid"
    with pytest.raises(LifecycleError) as failure:
        await h.engine.preview(h.target)
    assert failure.value.code == "runtimeEndpointUnavailable"
    h.runtime.delete_session.assert_not_called()
    await h.engine.close()


@pytest.mark.asyncio
async def test_restarted_delete_is_unknown_and_requires_two_read_only_absence_checks(harness):
    h = harness
    request = {**h.target, "operationId": str(uuid4()), "fingerprint": "a" * 64, "confirm": True}
    view = {"operationId": request["operationId"], **h.target, "name": "One", "state": "running", "stage": "agentkit_runtime"}
    h.engine.save_delete(request["operationId"], request, view)
    await h.engine.close()
    restarted = Engine("account", "owner", h.engine.directory, cloud_factory=lambda _: h.cloud, creations=h.engine.creations)
    body = {"operationId": request["operationId"], "region": "cn-beijing"}
    assert (await restarted.read_delete(body))["state"] == "unknown"
    missing = RuntimeError("private provider response")
    missing.error_code = "InvalidAgentKitRuntime.NotFound"
    h.cloud.get.side_effect = missing
    # A contradictory complete list must not confirm completion.
    assert (await restarted.read_delete(body))["state"] == "unknown"
    h.cloud.list_runtimes.side_effect = None
    h.cloud.list_runtimes.return_value = []
    assert (await restarted.read_delete(body))["state"] == "succeeded"
    h.cloud.call.assert_not_called()
    with pytest.raises(LifecycleError):
        await restarted.read_delete({**body, "region": "cn-shanghai"})
    await restarted.close()


@pytest.mark.asyncio
async def test_cleanup_failure_never_deletes_runtime(harness):
    h = harness
    h.cleanup.delete_resources.side_effect = RuntimeError("private cleanup diagnostics")
    preview = await h.engine.preview(h.target)
    request = {**h.target, "operationId": str(uuid4()), "fingerprint": preview["fingerprint"], "confirm": True}
    await h.engine.start_delete(request)
    await h.engine.deleting[request["operationId"]]
    value = await h.engine.read_delete({"operationId": request["operationId"], "region": "cn-beijing"})
    assert value["state"] == "unknown"
    assert "private" not in str(value)
    h.cloud.call.assert_not_called()
    await h.engine.close()


@pytest.mark.asyncio
async def test_creation_routes_reuse_real_journal_and_original_identity(tmp_path, monkeypatch):
    import sys
    import frontend.server.mpa_creation as routes
    from veadk.integrations.mpa.managed.tasks import owner_key

    # ASGI routes and CreationTasks are real; only name discovery and the cloud
    # runner are mocked so the test cannot provision any cloud resources.
    monkeypatch.setenv("VOLCENGINE_ACCESS_KEY", "TEST_AK")
    monkeypatch.setenv("VOLCENGINE_SECRET_KEY", "TEST_SK")
    monkeypatch.setattr(routes, "check_creation_name", AsyncMock(return_value="account"))
    engine = Engine("account", "owner", tmp_path)
    engine.creations.command = lambda: [sys.executable, "-c", "import time; time.sleep(30)"]
    body = {"requestId": str(uuid4()), "region": "cn-beijing", "name": "my-mpa", "description": "Test", "maxInstance": 20,
            "businessWorkspaceMode": "per-agent", "businessDatabaseName": "aidb", "openvikingMode": "manual",
            "openvikingUrl": "https://ov.invalid", "openvikingResourceId": "ov-one", "openvikingApiKey": "PRIVATE_OV_KEY"}
    assert (await engine.dispatch("config", {"region": "cn-beijing"}))["configured"]
    assert (await engine.dispatch("checkName", {k: body[k] for k in ("requestId", "region", "name")}))["available"]
    first = await engine.dispatch("create", body)
    again = await engine.dispatch("create", body)
    assert again["taskId"] == first["taskId"]
    assert "PRIVATE_OV_KEY" not in str(first)
    assert b"PRIVATE_OV_KEY" not in (tmp_path / "creations.sqlite3").read_bytes()
    assert len((await engine.dispatch("listCreations", {"region": "cn-beijing"}))["tasks"]) == 1
    assert not (await engine.dispatch("listCreations", {"region": "cn-shanghai"}))["tasks"]
    with pytest.raises(LifecycleError):
        await engine.dispatch("readCreation", {"taskId": first["taskId"], "region": "cn-shanghai"})
    with pytest.raises(LifecycleError):
        await engine.dispatch("create", {**body, "name": "changed-name"})
    cancelled = await engine.dispatch("cancelCreation", {"taskId": first["taskId"], "region": "cn-beijing"})
    assert cancelled["state"] == "cancelled"
    resumed = await engine.dispatch("create", body)
    assert resumed["taskId"] == first["taskId"]
    assert resumed["visitedStages"] == ["queued"]
    await engine.creations.cancel(owner_key("owner"), first["taskId"])
    # Simulate a supervisor process that disappeared between app launches.
    engine.creations._update(first["taskId"], state="running", supervisor=99999999)
    read = await engine.dispatch("readCreation", {"taskId": first["taskId"], "region": "cn-beijing"})
    assert read["state"] == "failed"
    recovered = await engine.dispatch("create", body)
    assert recovered["taskId"] == first["taskId"]
    await engine.close()


@pytest.mark.asyncio
@pytest.mark.parametrize("shared", [True, False])
async def test_engine_uses_studio_cleanup_ownership_and_retention_rules(harness, shared):
    from veadk.integrations.mpa.managed.database import agent_suffix
    from veadk.integrations.mpa.managed.deletion import OwnedResourceCleanup

    h = harness
    h.record["ToolId"] = "t-one"
    h.record["ProjectName"] = "default"
    h.record["Envs"].append({"Key": "SKILL_SPACE_ID", "Value": "ss-one"})
    entry = SimpleNamespace(record={"runtime_id": "r-one", "worker_id": "t-one", "worker_managed": True,
                                    "skill_space_id": "ss-one", "skill_space_managed": True})
    entry.read = AsyncMock(side_effect=lambda: deepcopy(entry.record))
    entry.save = AsyncMock(side_effect=lambda value: setattr(entry, "record", deepcopy(value)))
    entry.other_records = AsyncMock(return_value=[{"worker_id": "t-one", "skill_space_id": "ss-one"}] if shared else [])
    tags = [{"Key": "managed_by", "Value": "mpa-deployment"},
            {"Key": "mpa_agent_key", "Value": agent_suffix("account", "cn-beijing", "mi-one")}]
    worker = SimpleNamespace(get=AsyncMock(return_value={"ToolId": "t-one", "Tags": tags, "ProjectName": "default"}), delete=AsyncMock())
    h.cloud.get_skill_space = AsyncMock(return_value={"Id": "ss-one", "Tags": tags, "ProjectName": "default"})
    h.cloud.delete_skill_space = AsyncMock()

    @asynccontextmanager
    async def cleanup(**kwargs):
        yield OwnedResourceCleanup(entry=entry, cloud=h.cloud, worker=worker, account="account",
                                   region=kwargs["region"], agent_id=kwargs["agent_id"], runtime=kwargs["runtime"])

    h.engine.cleanup = cleanup
    preview = await h.engine.preview(h.target)
    if shared:
        assert len(preview["retainedResources"]) == 2
        assert all(item["reason"] == "shared" for item in preview["retainedResources"])
    else:
        assert [item["stage"] for item in preview["cleanupPlan"]] == ["runtime_sessions", "worker_template", "skill_space", "agentkit_runtime"]
    request = {**h.target, "operationId": str(uuid4()), "fingerprint": preview["fingerprint"], "confirm": True}
    await h.engine.start_delete(request)
    await h.engine.deleting[request["operationId"]]
    result = await h.engine.read_delete({"operationId": request["operationId"], "region": "cn-beijing"})
    assert result["state"] == "succeeded"
    if shared:
        worker.delete.assert_not_called()
        h.cloud.delete_skill_space.assert_not_called()
    else:
        worker.delete.assert_awaited_once_with("t-one")
        h.cloud.delete_skill_space.assert_awaited_once_with("ss-one")
        assert entry.record["deletion"]["worker_template"] == "deleted"
        assert entry.record["deletion"]["skill_space"] == "deleted"
    assert entry.record["state"] == "deleted"
    await h.engine.close()


def test_stage_journal_captures_fast_transitions_restarts_and_retry(tmp_path):
    import json
    import os
    import sqlite3
    from engine import DesktopCreationTasks
    from veadk.integrations.mpa.managed.tasks import CreationTasks, TaskError

    path = tmp_path / 'stages.sqlite3'
    # Upgrade an existing journal without inventing previous steps.
    legacy = CreationTasks(path)
    with legacy.db() as db:
        db.execute("INSERT INTO tasks(id,owner,request,payload,state,stage,supervisor,updated) VALUES(?,?,?,?,?,?,?,?)",
                   ('old', 'owner', 'old-request', '{}', 'succeeded', 'verifying', os.getpid(), 0))
    tasks = DesktopCreationTasks(path)
    assert 'visitedStages' not in tasks.get('owner', 'old')
    with tasks.db() as db:
        db.execute("INSERT INTO tasks(id,owner,request,payload,state,stage,supervisor,updated) VALUES(?,?,?,?,?,?,?,?)",
                   ('new', 'owner', 'new-request', '{}', 'running', 'queued', os.getpid(), 0))
    # A different connection models writes from the supervisor, between UI polls.
    with sqlite3.connect(path) as db:
        for stage in ['checking', 'iam_role', 'worker', 'deploying', 'verifying']:
            db.execute('UPDATE tasks SET stage=? WHERE id=?', (stage, 'new'))
        db.execute("UPDATE tasks SET state='succeeded' WHERE id='new'")
    restarted = DesktopCreationTasks(path)
    expected = ['queued', 'checking', 'iam_role', 'worker', 'deploying', 'verifying']
    assert restarted.get('owner', 'new')['visitedStages'] == expected
    with pytest.raises(TaskError):
        restarted.get('other-owner', 'new')
    with restarted.db() as db:
        db.execute("UPDATE tasks SET state='running', stage='queued' WHERE id='new'")
        db.execute("UPDATE tasks SET stage='worker' WHERE id='new'")
    assert restarted.get('owner', 'new')['visitedStages'] == ['queued', 'worker']
    # Stage tracking never stores payloads or cloud exception bodies.
    with restarted.db() as db:
        columns = [row[1] for row in db.execute('PRAGMA table_info(desktop_creation_stages)')]
    assert columns == ['id', 'task_id', 'stage']


@pytest.mark.asyncio
async def test_creation_failure_details_survive_real_supervisor_and_routes(tmp_path, monkeypatch):
    import json
    import sys
    import frontend.server.mpa_creation as routes

    monkeypatch.setenv('VOLCENGINE_ACCESS_KEY', 'TEST_AK')
    monkeypatch.setenv('VOLCENGINE_SECRET_KEY', 'TEST_SK')
    monkeypatch.setattr(routes, 'check_creation_name', AsyncMock(return_value='account'))
    engine = Engine('account', 'owner', tmp_path)
    diagnostic = dict(operation='get_worker', category='permission', attempt=2, outcome='failed',
                      errorCode='AccessDenied.Tool', requestId='0123456789abcdef0123456789abcdef',
                      exceptionType='HTTPError -> ApiError', httpStatus=403, responseType='application/json')
    events = [{'stage':'checking'}, {'stage':'worker'}, {'diagnostic':diagnostic}]
    runner = "import sys; sys.stdin.read(); " + '; '.join('print('+repr('MPA_EVENT '+json.dumps(event))+',flush=True)' for event in events) + '; sys.exit(1)'
    engine.creations.command = lambda: [sys.executable, '-c', runner]
    try:
        first = await engine.dispatch('create', {'requestId':str(uuid4()),'region':'cn-beijing','name':'my-mpa','description':'test','maxInstance':20,'openvikingMode':'auto'})
        for _ in range(100):
            value = await engine.dispatch('readCreation', {'region':'cn-beijing','taskId':first['taskId']})
            if value['state'] == 'failed': break
            await asyncio.sleep(.02)
        assert value['state'] == 'failed'
        assert value['latestDiagnostic'] == {'stage':'worker', **diagnostic}
        assert value['visitedStages'] == ['queued','checking','worker']
        listed = (await engine.dispatch('listCreations', {'region':'cn-beijing'}))['tasks'][0]
        assert listed['latestDiagnostic'] == value['latestDiagnostic']
        assert listed['visitedStages'] == value['visitedStages']
    finally:
        await engine.close()


@pytest.mark.asyncio
async def test_preview_failure_reports_stage_and_safe_http_details_without_mutation(harness):
    from frontend.server.mpa.runtime_client import MpaRuntimeError
    h = harness
    h.runtime.list_sessions.side_effect = MpaRuntimeError('runtime_request_failed', status_code=403,
                                                         request_id='0123456789abcdef0123456789abcdef')
    with pytest.raises(LifecycleError) as failure:
        await h.engine.preview(h.target)
    assert failure.value.code == 'deletePreviewFailed'
    diagnostic = failure.value.diagnostic
    assert diagnostic['stage'] == 'runtime_sessions'
    assert diagnostic['operation'] == 'list_sessions'
    assert diagnostic['category'] == 'permission'
    assert diagnostic['httpStatus'] == 403
    assert diagnostic['requestId'] == '0123456789abcdef0123456789abcdef'
    h.cloud.call.assert_not_called()
    h.runtime.delete_session.assert_not_called()
    h.cleanup.delete_resources.assert_not_called()
    with h.engine.db() as db:
        assert db.execute('SELECT count(*) FROM deletions').fetchone()[0] == 0
        assert db.execute('SELECT count(*) FROM diagnostics').fetchone()[0] == 1
    await h.engine.close()


@pytest.mark.asyncio
async def test_preview_unknown_exception_never_discloses_exception_text_or_credentials(harness):
    h = harness
    h.cleanup.preview.side_effect = TypeError('SECRET token password postgres://private')
    with pytest.raises(LifecycleError) as failure:
        await h.engine.preview(h.target)
    diagnostic = failure.value.diagnostic
    assert diagnostic['stage'] == 'managed_resources'
    assert diagnostic['exceptionType'] == 'TypeError'
    assert 'SECRET' not in str(diagnostic)
    assert 'postgres://' not in str(diagnostic)
    assert 'private' not in str(diagnostic)
    h.cloud.call.assert_not_called()
    await h.engine.close()


@pytest.mark.asyncio
async def test_deletion_directory_snapshot_is_private_and_refreshable(monkeypatch):
    from engine import DeletionCloud
    from veadk.integrations.mpa.managed.runtime import RuntimeCloud
    listing=AsyncMock(side_effect=[[{'RuntimeId':'one','Envs':[]}],[{'RuntimeId':'one'},{'RuntimeId':'new-reference'}]])
    monkeypatch.setattr(RuntimeCloud,'list_runtimes',listing)
    cloud=DeletionCloud(region='cn-beijing',credential_file='')
    assert cloud.cache_identity is True
    first=await cloud.list_runtimes()
    first[0]['Envs'].append({'Key':'modified','Value':'never-cached'})
    assert (await cloud.list_runtimes())[0]['Envs'] == []
    assert listing.await_count == 1
    cloud.invalidate_directory()
    assert len(await cloud.list_runtimes()) == 2
    assert listing.await_count == 2


@pytest.mark.asyncio
async def test_registry_timeout_preserves_actual_stage_and_does_not_accept_deletion(harness,monkeypatch):
    import engine as module
    h=harness
    @asynccontextmanager
    async def blocked(**kwargs):
        await asyncio.Event().wait()
        yield
    h.engine.cleanup=blocked
    monkeypatch.setattr(module,'PREVIEW_TIMEOUT',.02)
    with pytest.raises(LifecycleError) as failure:
        await h.engine.preview(h.target)
    assert failure.value.diagnostic['stage']=='registry'
    assert failure.value.diagnostic['operation']=='open_registry'
    assert failure.value.diagnostic['category']=='timeout'
    with h.engine.db() as db:assert db.execute('SELECT count(*) FROM deletions').fetchone()[0]==0
    h.cloud.call.assert_not_called()
    await h.engine.close()


@pytest.mark.asyncio
async def test_accepted_delete_invalidates_preview_directory_before_destructive_cleanup(harness, monkeypatch):
    from engine import DeletionCloud
    from veadk.integrations.mpa.managed.runtime import RuntimeCloud
    h=harness
    listings=AsyncMock(side_effect=lambda: [deepcopy(h.record)])
    monkeypatch.setattr(RuntimeCloud,'list_runtimes',listings)
    instances=[]
    def factory(_region):
        cloud=DeletionCloud(region='cn-beijing',credential_file='')
        cloud.account_id=AsyncMock(return_value='account')
        cloud.get=AsyncMock(side_effect=lambda _:deepcopy(h.record))
        cloud.call=AsyncMock()
        instances.append(cloud)
        return cloud
    h.engine.cloud_factory=factory
    async def cleanup():
        cloud=instances[-1]
        # A new Runtime reference could appear after the reviewed snapshot.
        assert cloud._directory is None
        listings.side_effect=lambda:[deepcopy(h.record),{'RuntimeId':'new-reference'}]
        assert len(await cloud.list_runtimes())==2
    h.cleanup.delete_resources.side_effect=cleanup
    preview=await h.engine.preview(h.target)
    body={**h.target,'operationId':str(uuid4()),'fingerprint':preview['fingerprint'],'confirm':True}
    await h.engine.start_delete(body)
    await h.engine.deleting[body['operationId']]
    result=await h.engine.read_delete({'operationId':body['operationId'],'region':'cn-beijing'})
    assert result['state']=='succeeded'
    assert len(instances)==3  # independent preview, acceptance recheck, execution check
    assert listings.await_count==4  # plus the forced fresh scan inside cleanup
    await h.engine.close()

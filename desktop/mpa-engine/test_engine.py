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

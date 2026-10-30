import asyncio
from copy import deepcopy
from datetime import datetime
import json
import os
from types import SimpleNamespace
from unittest.mock import AsyncMock

import pytest

import creation_runner as runner
from engine import DesktopCreationTasks
from veadk.integrations.mpa.managed.database import DeploymentError
from veadk.integrations.mpa.managed.diagnostics import diagnostic_scope, report_failure


ACCOUNT, REGION, RUNTIME = "2111111111", "cn-beijing", "r-test"
REQUEST = "0123456789abcdef0123456789abcdef"
CODE = "QuotaExceed.EndpointEachVpc"
CREATED, UPDATED = "2026-10-29T19:36:42+08:00", "2026-10-29T19:36:52+08:00"
START, END = runner.timestamp(CREATED) - 1, runner.timestamp(UPDATED) + 2


def event(name, source, when, *, params=None, response=None, code="", actor="ServiceRoleForAgentKit/agentkit-assume-role"):
    return {"event_detail": json.dumps({
        "EventName": name, "EventSource": source, "EventTime": when,
        "Region": REGION, "UserIdentity": {"AccountID": ACCOUNT, "PrincipalID": actor},
        "RequestParameters": json.dumps(params or {}),
        "ResponseElements": json.dumps(response or {}),
        "ErrorCode": code, "RequestID": REQUEST,
        "ErrorMessage": "PRIVATE_ERROR_WITH_PASSWORD",
    })}


def events():
    return [
        event("CreateRuntime", "agentkit", CREATED, params={"Name": "test", "Envs": "PRIVATE_KEY"},
              response={"Result": {"RuntimeId": RUNTIME}}, actor="caller"),
        event("CreateVpcEndpoint", "privatelink", UPDATED,
              params={"VpcId": "vpc-one", "ServiceId": "epsvc-one", "ClientToken": "operation-one"}, code=CODE),
    ]


def resolve(rows):
    return runner.audit_diagnostic(rows, runtime_id=RUNTIME, account=ACCOUNT, region=REGION, start=START, end=END)


def cloud(**changes):
    runtime = {"RuntimeId": RUNTIME, "Status": "Error", "StatusMessage": "", "CreatedAt": CREATED, "UpdatedAt": UPDATED}
    return SimpleNamespace(account=ACCOUNT, region=REGION, provider="volcengine", get=AsyncMock(return_value=runtime), **changes)


def deployer(fake):
    return runner.DiagnosticRuntimeDeployer(registry=None, databases=None, cloud=fake,
                                           region=REGION, shared_database_url="", timeout=2, interval=0)


def test_exact_runtime_receipt_correlates_privatelink_retries_without_exposing_payloads():
    rows = events()
    retry = deepcopy(rows[-1])
    detail = json.loads(retry["event_detail"])
    detail.update(EventTime="2026-10-29T19:36:43+08:00", RequestID="a" * 32)
    retry["event_detail"] = json.dumps(detail)
    actual = resolve([*rows, retry])
    assert actual["errorCode"] == CODE
    assert actual["requestId"] == REQUEST  # latest failure, not the earlier retry
    assert "PRIVATE" not in json.dumps(actual)


@pytest.mark.parametrize("change", ["account", "region", "time", "runtime", "concurrent", "dependency", "actor", "malformed"])
def test_unrelated_or_ambiguous_audit_records_are_not_attributed_to_this_runtime(change):
    rows = events()
    if change == "concurrent":
        rows.append(event("CreateRuntime", "agentkit", CREATED, response={"Result": {"RuntimeId": "r-other"}}))
    elif change == "dependency":
        extra = deepcopy(rows[-1]); value = json.loads(extra["event_detail"])
        value["RequestParameters"] = json.dumps({"VpcId": "vpc-one", "ServiceId": "epsvc-other", "ClientToken": "other"})
        extra["event_detail"] = json.dumps(value); rows.append(extra)
    elif change == "malformed":
        rows[0]["event_detail"] = "PRIVATE not JSON"
    else:
        index = 0 if change == "runtime" else 1
        value = json.loads(rows[index]["event_detail"])
        if change == "account": value["UserIdentity"]["AccountID"] = "other"
        if change == "region": value["Region"] = "cn-shanghai"
        if change == "time": value["EventTime"] = "2026-10-29T18:36:42+08:00"
        if change == "actor": value["UserIdentity"]["PrincipalID"] = "another-service/session"
        if change == "runtime": value["ResponseElements"] = json.dumps({"Result": {"RuntimeId": "r-other"}})
        rows[index]["event_detail"] = json.dumps(value)
    assert resolve(rows) is None


def test_direct_resource_error_works_without_inferring_a_dependency():
    rows = [event("UpdateRuntime", "agentkit", UPDATED, params={"RuntimeId": RUNTIME}, code="AccessDenied.Runtime")]
    assert resolve(rows)["errorCode"] == "AccessDenied.Runtime"


@pytest.mark.parametrize("message", [
    {"Code": CODE, "RequestId": REQUEST, "Message": "PRIVATE"},
    json.dumps({"ResponseMetadata": {"Error": {"Code": CODE, "Message": "PRIVATE"}, "RequestId": REQUEST}}),
    CODE + ": PRIVATE",
])
def test_runtime_status_identifiers_do_not_forward_raw_messages(message):
    value = runner.runtime_diagnostic({"StatusMessage": message})
    assert value["errorCode"] == CODE
    assert "PRIVATE" not in json.dumps(value)


@pytest.mark.parametrize("message", ["Cloud deployment failed", "SECRET_TOKEN", "password=PRIVATE", "{invalid}", [CODE]])
def test_unstructured_status_is_not_treated_as_a_provider_error_code(message):
    assert runner.runtime_diagnostic({"StatusMessage": message}) is None


@pytest.mark.asyncio
async def test_readiness_failure_is_enriched_and_survives_the_real_journal(tmp_path, monkeypatch):
    fake = cloud()
    lookup = AsyncMock(return_value=events())
    monkeypatch.setattr(runner, "audit_events", lookup)
    with pytest.raises(DeploymentError) as failure:
        await deployer(fake).wait_platform(RUNTIME)
    assert str(failure.value) == runner.FAILED
    emitted = []
    with diagnostic_scope(emitted.append):
        value = report_failure("provision", failure.value)
    assert value["operation"] == "wait_runtime"
    assert value["errorCode"] == CODE
    assert value["requestId"] == REQUEST
    assert value["exceptionType"] == "RuntimeError"
    tasks = DesktopCreationTasks(tmp_path / "creation.sqlite3")
    with tasks.db() as db:
        db.execute("INSERT INTO tasks(id,owner,request,payload,state,stage,supervisor,updated) VALUES(?,?,?,?,?,?,?,?)",
                   ("task", "owner", "request", "{}", "failed", "verifying", os.getpid(), 0))
    tasks.record_diagnostic("task", value)
    saved = DesktopCreationTasks(tasks.path).get("owner", "task")
    assert saved["latestDiagnostic"]["errorCode"] == CODE
    assert saved["latestDiagnostic"]["requestId"] == REQUEST
    assert "PRIVATE" not in json.dumps(saved)
    assert lookup.await_count == 1


@pytest.mark.asyncio
async def test_runtime_status_takes_precedence_over_audit(monkeypatch):
    fake = cloud()
    fake.get.return_value["StatusMessage"] = {"Code": CODE, "RequestId": REQUEST}
    lookup = AsyncMock()
    monkeypatch.setattr(runner, "audit_events", lookup)
    with pytest.raises(DeploymentError) as failure:
        await deployer(fake).wait_platform(RUNTIME)
    assert getattr(failure.value, "_mpa_creation_diagnostic")["errorCode"] == CODE
    lookup.assert_not_called()


@pytest.mark.asyncio
@pytest.mark.parametrize("outcome", ["denied", "missing", "timeout", "wrong_runtime"])
async def test_lookup_failures_preserve_original_error_without_adding_a_false_cause(monkeypatch, outcome):
    fake = cloud()
    async def lookup(*_args):
        if outcome == "denied": raise PermissionError("PRIVATE credential body")
        if outcome == "timeout": await asyncio.sleep(60)
        return []
    if outcome == "wrong_runtime":
        fake.get.side_effect = [fake.get.return_value, {"RuntimeId": "r-other", "Status": "Error"}]
    monkeypatch.setattr(runner, "audit_events", lookup)
    monkeypatch.setattr(runner, "LOOKUP_TIMEOUT", 0.01)
    with pytest.raises(DeploymentError) as failure:
        await deployer(fake).wait_platform(RUNTIME)
    assert str(failure.value) == runner.FAILED
    assert not getattr(failure.value, "_mpa_creation_diagnostic", None)


@pytest.mark.asyncio
async def test_delayed_audit_delivery_is_retried_read_only(monkeypatch):
    fake = cloud()
    lookup = AsyncMock(side_effect=[[], events()])
    monkeypatch.setattr(runner, "audit_events", lookup)
    monkeypatch.setattr(runner.asyncio, "sleep", AsyncMock())
    value = await runner.diagnose_runtime(fake, RUNTIME, START)
    assert value["errorCode"] == CODE
    assert lookup.await_count == 2
    fake.get.assert_awaited_once_with(RUNTIME)


@pytest.mark.asyncio
async def test_ready_runtimes_and_cancellation_do_not_trigger_lookup(monkeypatch):
    fake = cloud()
    fake.get.return_value["Status"] = "Ready"
    lookup = AsyncMock()
    monkeypatch.setattr(runner, "audit_events", lookup)
    assert (await deployer(fake).wait_platform(RUNTIME))["Status"] == "Ready"
    fake.get.side_effect = asyncio.CancelledError()
    with pytest.raises(asyncio.CancelledError):
        await deployer(fake).wait_platform(RUNTIME)
    lookup.assert_not_called()


@pytest.mark.asyncio
@pytest.mark.parametrize("tokens", [["next", ""], ["repeat", "repeat"], [str(i) for i in range(runner.MAX_PAGES)]])
async def test_audit_sdk_uses_verified_credentials_and_bounded_complete_pagination(monkeypatch, tokens):
    import volcenginesdkcloudtrail as sdk
    calls = []
    responses = iter(tokens)
    credential = SimpleNamespace(access_key_id="PRIVATE_AK", secret_access_key="PRIVATE_SK", session_token="PRIVATE_STS")
    fake = cloud(_credentials=lambda: credential)
    def client(api):
        config = api.configuration
        assert (config.ak, config.sk, config.session_token, config.region) == (credential.access_key_id, credential.secret_access_key, credential.session_token, REGION)
        def lookup(request, **kwargs):
            assert request.start_time == START and request.end_time == END
            assert request.lookup_conditions[0].lookup_condition_key == "ReadOnly"
            assert request.lookup_conditions[0].lookup_condition_value == "false"
            assert kwargs["_request_timeout"] == (2, 4)
            calls.append(request.next_token)
            return SimpleNamespace(to_dict=lambda: {"trails": [], "next_token": next(responses)})
        return SimpleNamespace(lookup_events=lookup)
    monkeypatch.setattr(sdk, "CLOUDTRAILApi", client)
    value = await runner.audit_events(fake, START, END)
    assert value == ([] if tokens[-1] == "" else None)
    assert len(calls) == len(tokens)


def test_creation_command_uses_the_bundled_runner_in_isolated_mode(tmp_path):
    from pathlib import Path
    tasks = DesktopCreationTasks(tmp_path / "creation.sqlite3")
    assert tasks.command()[1] == "-I"
    assert Path(tasks.command()[2]).resolve() == Path(runner.__file__).resolve()

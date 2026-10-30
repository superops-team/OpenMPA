"""Failure-path integration tests. Every cloud transport is a local fake."""

import asyncio
from copy import deepcopy
import json
from types import SimpleNamespace
from unittest.mock import AsyncMock

import pytest
from sqlalchemy.exc import DBAPIError

import creation_runner as runner
from test_creation_runner import ACCOUNT, REGION, REQUEST, CREATED, UPDATED, START, END, event
from veadk.integrations.mpa.managed import gateway_cloud, network_cloud, pg_cloud, runtime, service
from veadk.integrations.mpa.managed import runner as studio_runner
from veadk.integrations.mpa.managed.config import Worker
from veadk.integrations.mpa.managed.database import DeploymentError, agent_suffix
from veadk.integrations.mpa.managed.diagnostics import diagnostic_scope, report_failure
from veadk.integrations.mpa.managed.worker import WorkerCloud, ensure_worker

TOOL = "t-test"
CODE = "ImagePullFailed"
FAILED = "Worker is not ready; inspect it and retry the same agent"


def cloud():
    return SimpleNamespace(account=ACCOUNT, region=REGION, provider="volcengine")


@pytest.fixture
def installed(monkeypatch):
    # Record module bindings so installation cannot leak into another test.
    for module, key in ((service, "WorkerCloud"), (runtime, "NetworkCloud"),
                        (service, "GatewayCloud"), (pg_cloud, "PGCloud"),
                        (service, "OpenVikingCloud"),
                        (service, "AgentRuntimeDeployer"), (studio_runner, "provision")):
        monkeypatch.setattr(module, key, getattr(module, key))
    runner.install_diagnostics()


def tool(status="Error"):
    return {"ToolId": TOOL, "Status": status, "ProjectName": "default",
            "CreatedAt": CREATED, "UpdatedAt": UPDATED,
            "Tags": [{"Key": "managed_by", "Value": "mpa-deployment"},
                     {"Key": "mpa_agent_key", "Value": agent_suffix(ACCOUNT, REGION, "test")}],
            "Envs": [{"Key": "PRIVATE_PASSWORD", "Value": "PRIVATE"}]}


@pytest.mark.asyncio
async def test_real_worker_creation_keeps_receipt_and_enriches_terminal_failure(monkeypatch, installed, tmp_path):
    from engine import DesktopCreationTasks
    calls, record = [], {}

    async def call(_self, method, request_type, **values):
        calls.append(method)
        if method == "list_tools": return {"Tools": []}
        if method == "create_tool": return {"ToolId": TOOL}
        assert method == "get_tool" and values == {"ToolId": TOOL}
        return tool()

    monkeypatch.setattr(WorkerCloud, "call", call)
    release = AsyncMock(return_value=runner.diagnostic(CODE, REQUEST, "worker_state"))
    monkeypatch.setattr(runner, "worker_release_diagnostic", release)
    audit = AsyncMock()
    monkeypatch.setattr(runner, "audit_events", audit)
    entry = SimpleNamespace(read=AsyncMock(side_effect=lambda: deepcopy(record)),
                            save=AsyncMock(side_effect=lambda value: record.update(value)))

    async def provision(*, progress):
        progress("worker")
        await ensure_worker(entry, service.WorkerCloud(cloud()), Worker(image="example/image:tag"),
                            account=ACCOUNT, region=REGION, agent_id="test")

    with pytest.raises(DeploymentError, match="Worker is not ready") as failure:
        await runner.enriched_provision(provision)
    detail = report_failure("provision", failure.value)
    assert detail["operation"] == "worker_state" and detail["errorCode"] == CODE
    assert detail["requestId"] == REQUEST and record["worker_id"] == TOOL
    assert calls == ["list_tools", "create_tool", "get_tool"]  # no new write during lookup
    audit.assert_not_called()
    tasks = DesktopCreationTasks(tmp_path / "creation.sqlite3")
    with tasks.db() as db:
        db.execute("INSERT INTO tasks(id,owner,request,payload,state,stage,supervisor,updated) VALUES(?,?,?,?,?,?,?,?)",
                   ("task", "owner", "request", "{}", "failed", "worker", 1, 0))
    tasks.record_diagnostic("task", detail)
    saved = DesktopCreationTasks(tasks.path).get("owner", "task")
    assert saved["latestDiagnostic"]["errorCode"] == CODE
    assert "PRIVATE" not in json.dumps(saved)


@pytest.mark.asyncio
@pytest.mark.parametrize("failure", ["denied", "timeout", "ambiguous", "cancelled", "ownership", "recovered", "stage_changed"])
async def test_worker_diagnostic_is_best_effort_and_never_replaces_other_failures(monkeypatch, failure):
    audit = AsyncMock(return_value=[])
    release = AsyncMock(return_value=None)
    monkeypatch.setattr(runner, "audit_events", audit)
    monkeypatch.setattr(runner, "worker_release_diagnostic", release)
    monkeypatch.setattr(runner, "LOOKUP_TIMEOUT", .01)
    original = DeploymentError(FAILED if failure not in {"ownership", "stage_changed", "recovered"}
                               else "Worker ownership does not match this agent")
    if failure == "denied": audit.side_effect = PermissionError("PRIVATE")
    if failure == "timeout":
        async def wait(*args): await asyncio.sleep(30)
        audit.side_effect = wait

    async def provision(*, progress):
        progress("worker")
        runner.observe(cloud(), tool(), resource_id=TOOL, id_key="ToolId", source="agentkit",
                       operation="worker_state", kind="worker")
        if failure == "cancelled": raise asyncio.CancelledError()
        if failure == "stage_changed": progress("deploying")
        if failure == "recovered":
            runner.observe(cloud(), tool("Ready"), resource_id=TOOL, id_key="ToolId", source="agentkit",
                           operation="worker_state", kind="worker")
        raise original

    if failure == "cancelled":
        with pytest.raises(asyncio.CancelledError): await runner.enriched_provision(provision)
    else:
        with pytest.raises(DeploymentError) as actual: await runner.enriched_provision(provision)
        assert actual.value is original
        assert not getattr(original, "_mpa_creation_diagnostic", None)
    if failure in {"cancelled", "ownership", "stage_changed", "recovered"}:
        audit.assert_not_called()
        release.assert_not_called()
    assert runner._trace.get() is None


@pytest.mark.parametrize("change", ["none", "tool", "concurrent", "function", "actor", "release"])
def test_worker_faas_audit_requires_exact_tool_receipt_and_function(change):
    rows = [event("CreateTool", "agentkit", CREATED, response={"Result": {"ToolId": TOOL}}),
            event("CreateFunction", "vefaas", UPDATED,
                  params={"Name": "agentkit-builtin-tools-" + TOOL}, code=CODE)]
    if change == "tool": rows[0] = event("CreateTool", "agentkit", CREATED, response={"Result": {"ToolId": "t-other"}})
    if change == "concurrent": rows.append(event("CreateTool", "agentkit", CREATED, response={"Result": {"ToolId": "t-other"}}))
    if change == "function": rows[1] = event("CreateFunction", "vefaas", UPDATED, params={"Name": "agentkit-builtin-tools-t-other"}, code=CODE)
    if change == "actor": rows[1] = event("CreateFunction", "vefaas", UPDATED, params={"Name": "agentkit-builtin-tools-" + TOOL}, code=CODE, actor="unrelated")
    if change == "release":
        rows[1] = event("CreateFunction", "vefaas", CREATED, params={"Name": "agentkit-builtin-tools-" + TOOL}, response={"Result": {"Id": "function-one"}})
        rows.append(event("ReleaseFunction", "vefaas", UPDATED, params={"FunctionId": "function-one"}, code=CODE))
    detail = runner.audit_diagnostic(rows, runtime_id=TOOL, account=ACCOUNT, region=REGION, start=START,
                                    end=END, resource_keys=("ToolId",), operation="worker_state")
    assert (detail or {}).get("errorCode") == (CODE if change in {"none", "release"} else None)


@pytest.mark.asyncio
async def test_worker_uses_audit_when_release_details_are_unavailable(monkeypatch):
    observation = runner.Observation(cloud(), TOOL, ("ToolId",), "agentkit", "worker_state", "worker",
                                     {"CreatedAt": CREATED, "UpdatedAt": UPDATED}, None)
    monkeypatch.setattr(runner, "worker_release_diagnostic", AsyncMock(side_effect=PermissionError()))
    rows = [event("UpdateTool", "agentkit", UPDATED, params={"ToolId": TOOL}, code="QuotaExceed.Tool")]
    monkeypatch.setattr(runner, "audit_events", AsyncMock(return_value=rows))
    assert (await runner.diagnose_observation(observation, START))["errorCode"] == "QuotaExceed.Tool"


@pytest.mark.asyncio
@pytest.mark.parametrize("kind,operation,source,key,method,row,params,message", [
    ("vpc", "describe_vpc_attributes", "vpc", "VpcId", "describe_vpc_attributes",
     {"vpc_id": "vpc-test", "status": "Failed", "account_id": ACCOUNT}, {"vpc_id": "vpc-test"},
     "Registered vpc is unavailable; repair the resource before retrying"),
    ("subnet", "describe_subnet_attributes", "vpc", "SubnetId", "describe_subnet_attributes",
     {"subnet_id": "subnet-test", "status": "Failed", "account_id": ACCOUNT}, {"subnet_id": "subnet-test"},
     "Registered subnet is unavailable; repair the resource before retrying"),
    ("gateway", "GetGateway", "apig", "Id", "GetGateway",
     {"Gateway": {"Id": "gateway-test", "Status": "FAILED", "Region": REGION}}, {"Id": "gateway-test"},
     "Shared gateway is unavailable; repair the existing resource"),
    ("im_gateway", "GetIMChannelGatewayStatus", "apig", "GatewayId", "GetIMChannelGatewayStatus",
     {"Status": "FAILED"}, {"GatewayId": "gateway-test"},
     "IM Gateway failed; inspect the registered service"),
    ("postgres", "aidap_describe_workspace_detail", "aidap", "WorkspaceId", "describe_workspace_detail",
     {"workspace": {"workspace_id": "workspace-test", "workspace_status": "CreateFailed", "account_id": ACCOUNT}},
     {"workspace_id": "workspace-test"},
     "PostgreSQL Workspace creation failed; correct AIDAP permissions or configuration before retrying"),
])
async def test_other_resource_adapters_recover_exact_audit_error(monkeypatch, installed,
        kind, operation, source, key, method, row, params, message):
    bound = cloud()
    bound._credentials = lambda: None
    # Use a real bound method as the verified credential source.
    class Source:
        account, region, provider = ACCOUNT, REGION, "volcengine"
        def _credentials(self): return None
    bound = Source()
    resource_id = next(iter(params.values()))
    action = {"vpc": "CreateVpc", "subnet": "CreateSubnet", "gateway": "CreateGateway",
              "im_gateway": "CreateIMChannelGateway", "postgres": "CreateWorkspace"}[kind]
    lookup = AsyncMock(return_value=[event(action, source, UPDATED,
                                         params={key: resource_id}, code="QuotaExceed.Resource")])
    monkeypatch.setattr(runner, "audit_events", lookup)
    if source == "vpc":
        monkeypatch.setattr(network_cloud.NetworkCloud, "call", AsyncMock(return_value=row))
        adapter = runtime.NetworkCloud(region=REGION, credentials=bound._credentials)
    elif source == "apig":
        monkeypatch.setattr(gateway_cloud.GatewayCloud, "call", AsyncMock(return_value=row))
        adapter = service.GatewayCloud(bound)
    else:
        # The installed PG class inherits the original SDK adapter.
        monkeypatch.setattr(pg_cloud.PGCloud.__bases__[0], "call", AsyncMock(return_value=row))
        adapter = pg_cloud.PGCloud(region=REGION, credentials=bound._credentials)

    async def provision(*, progress):
        progress(kind)
        if source == "apig": await adapter.call(method, params)
        else: await adapter.call(method, "Request", **params)
        raise DeploymentError(message)

    # Audit timestamps are fixed to the original failure rather than test wall time.
    monkeypatch.setattr(runner, "failure_window", lambda *_: (START, END))
    with pytest.raises(DeploymentError) as failure: await runner.enriched_provision(provision)
    detail = report_failure("provision", failure.value)
    assert detail["errorCode"] == "QuotaExceed.Resource" and detail["operation"] == operation
    assert detail["requestId"] == REQUEST
    lookup.assert_awaited_once()


@pytest.mark.asyncio
async def test_suppressed_database_exception_retains_only_sqlstate():
    class DriverError(Exception): sqlstate = "28P01"
    async def provision(*, progress):
        progress("admin_database")
        try:
            raise DBAPIError("PRIVATE SQL", {"password": "PRIVATE"}, DriverError("PRIVATE URL"))
        except DBAPIError:
            raise DeploymentError("Database preparation failed") from None
    with pytest.raises(DeploymentError) as failure: await runner.enriched_provision(provision)
    detail = report_failure("provision", failure.value)
    assert detail["errorCode"] == "PostgreSQL.28P01" and detail["operation"] == "prepare_database"
    assert "PRIVATE" not in json.dumps(detail)


@pytest.mark.asyncio
@pytest.mark.parametrize("change", ["none", "name", "ambiguous", "id", "status", "time", "secret"])
async def test_faas_lookup_is_read_only_and_requires_exact_failed_release(monkeypatch, change):
    import volcenginesdkvefaas as sdk
    bound = cloud()
    bound._credentials = lambda: SimpleNamespace(access_key_id="PRIVATE_AK", secret_access_key="PRIVATE_SK", session_token="PRIVATE_STS")
    items = {"total": 1, "items": [{"id": "function-one", "name": "agentkit-builtin-tools-" + TOOL}]}
    release = {"function_id": "function-one", "status": "failed", "start_time": CREATED,
               "error_code": CODE, "failed_instance_logs": "PRIVATE", "status_message": "PRIVATE"}
    if change == "name": items["items"][0]["name"] = "unrelated"
    if change == "ambiguous": items["total"] = 2
    if change == "id": release["function_id"] = "unrelated"
    if change == "status": release["status"] = "done"
    if change == "time": release["start_time"] = "2026-01-01T00:00:00+08:00"
    if change == "secret": release["error_code"] = "SECRET_PASSWORD"
    calls = []
    def client(api):
        assert api.configuration.ak == "PRIVATE_AK" and api.configuration.region == REGION
        def listing(request, **kwargs):
            assert request.filters[0].values == ["agentkit-builtin-tools-" + TOOL]
            assert request.page_size == 2 and kwargs["_request_timeout"] == (2, 3)
            calls.append("ListFunctions")
            return SimpleNamespace(to_dict=lambda: items)
        def status(request, **kwargs):
            assert request.function_id == "function-one"
            calls.append("GetReleaseStatus")
            return SimpleNamespace(to_dict=lambda: release)
        return SimpleNamespace(list_functions=listing, get_release_status=status)
    monkeypatch.setattr(sdk, "VEFAASApi", client)
    detail = await runner.worker_release_diagnostic(bound, TOOL, START, END)
    assert (detail or {}).get("errorCode") == (CODE if change == "none" else None)
    assert "PRIVATE" not in json.dumps(detail)
    assert set(calls) <= {"ListFunctions", "GetReleaseStatus"}


@pytest.mark.parametrize("source,key,name,params", [
    ("vefaas", "ToolId", "CreateFunction", {"Name": "agentkit-builtin-tools-" + TOOL}),
    ("privatelink", "ToolId", "CreateVpcEndpoint", {"VpcId": "vpc", "ServiceId": "service", "ClientToken": "token"}),
])
def test_successful_dependency_retry_cannot_be_reported_as_current_root_cause(source, key, name, params):
    rows = [event("CreateTool", "agentkit", CREATED, response={"Result": {key: TOOL}}),
            event(name, source, CREATED, params=params, code="QuotaExceed.Resource"),
            event(name, source, UPDATED, params=params)]
    assert runner.audit_diagnostic(rows, runtime_id=TOOL, account=ACCOUNT, region=REGION, start=START,
                                   end=END, resource_keys=(key,), operation="worker_state") is None


@pytest.mark.asyncio
@pytest.mark.parametrize("method,qualifiers,result", [
    ("describe_branches", {"BranchId": "main-one"},
     {"branches": [{"branch_name": "main", "branch_id": "main-one", "branch_status": "Failed"},
                   {"branch_name": "other", "branch_id": "other", "branch_status": "Failed"}]}),
    ("describe_computes", {"BranchId": "main-one", "ComputeId": "primary-one"},
     {"computes": [{"compute_id": "primary-one", "compute_role": "primary", "compute_status": "Failed"},
                   {"compute_id": "secondary", "compute_role": "readonly", "compute_status": "Failed"}]}),
])
async def test_database_subresource_errors_require_selected_branch_and_compute(monkeypatch, installed, method, qualifiers, result):
    class Source:
        account, region, provider = ACCOUNT, REGION, "volcengine"
        def _credentials(self): return None
    adapter = pg_cloud.PGCloud(region=REGION, credentials=Source()._credentials)
    monkeypatch.setattr(pg_cloud.PGCloud.__bases__[0], "call", AsyncMock(return_value=result))
    monkeypatch.setattr(runner, "failure_window", lambda *_: (START, END))
    params = {"WorkspaceId": "workspace-test", **qualifiers}
    wrong = {**params, "BranchId": "other"}
    action = "CreateCompute" if method == "describe_computes" else "CreateBranch"
    monkeypatch.setattr(runner, "audit_events", AsyncMock(return_value=[
        event(action, "aidap", UPDATED, params=wrong, code="WrongBranchFailed"),
        event(action, "aidap", UPDATED, params=params, code="SelectedResourceFailed"),
    ]))
    async def provision(*, progress):
        progress("business_workspace")
        await adapter.call(method, "Request", workspace_id="workspace-test", branch_id="main-one")
        pg_cloud.check_ready("Failed")
    with pytest.raises(DeploymentError) as failure: await runner.enriched_provision(provision)
    detail = report_failure("provision", failure.value)
    assert detail["errorCode"] == "SelectedResourceFailed"
    assert detail["operation"] == pg_cloud.METHOD_OPERATIONS[method]


@pytest.mark.asyncio
async def test_structured_status_precedes_audit_and_byteplus_never_uses_domestic_audit(monkeypatch):
    source = cloud()
    source.provider = "byteplus"
    monkeypatch.setattr(runner, "audit_events", AsyncMock())
    observation = runner.Observation(source, "gateway", ("Id",), "apig", "GetGateway", "gateway", {},
                                     runner.diagnostic("GatewayCreateFailed", REQUEST, "GetGateway"))
    assert (await runner.diagnose_observation(observation, START))["errorCode"] == "GatewayCreateFailed"
    observation.detail = None
    assert await runner.diagnose_observation(observation, START) is None
    runner.audit_events.assert_not_called()


def test_shared_vpc_reference_in_unrelated_subnet_failure_is_not_a_vpc_root_cause():
    rows = [event("CreateSubnet", "vpc", UPDATED, params={"VpcId": "vpc-test"}, code="QuotaExceed.Subnet")]
    assert runner.audit_diagnostic(rows, runtime_id="vpc-test", account=ACCOUNT, region=REGION,
                                   start=START, end=END, resource_keys=("VpcId",), source="vpc",
                                   operation="describe_vpc_attributes") is None


@pytest.mark.asyncio
@pytest.mark.parametrize("kind", ["worker", "runtime"])
@pytest.mark.parametrize("timeout", [.08, .5])
async def test_diagnostics_near_creation_deadline_preserve_original_failure(monkeypatch, kind, timeout):
    profile = SimpleNamespace(managed=SimpleNamespace(timeout_seconds=timeout))
    message = FAILED if kind == "worker" else runner.FAILED
    original = DeploymentError(message)
    async def slow(*args): await asyncio.sleep(60)
    monkeypatch.setattr(runner, "diagnose_observation", slow)
    monkeypatch.setattr(runner, "diagnose_runtime", slow)
    monkeypatch.setattr(runner.AgentRuntimeDeployer, "wait_platform", AsyncMock(side_effect=original))
    deployer = runner.DiagnosticRuntimeDeployer(registry=None, databases=None, cloud=cloud(),
                                                region=REGION, shared_database_url="")
    async def provision(profile, *, progress):
        progress("worker" if kind == "worker" else "verifying")
        await asyncio.sleep(.05)
        if kind == "worker":
            runner.observe(cloud(), tool(), resource_id=TOOL, id_key="ToolId", source="agentkit",
                           operation="worker_state", kind="worker")
            raise original
        await deployer.wait_platform("r-test")
    with pytest.raises(DeploymentError) as failure:
        await asyncio.wait_for(runner.enriched_provision(provision, profile), profile.managed.timeout_seconds)
    assert failure.value is original


@pytest.mark.asyncio
async def test_user_cancellation_during_lookup_is_not_swallowed(monkeypatch):
    entered = asyncio.Event()
    async def slow(*args):
        entered.set()
        await asyncio.sleep(60)
    monkeypatch.setattr(runner, "diagnose_observation", slow)
    async def provision(*, progress):
        progress("worker")
        runner.observe(cloud(), tool(), resource_id=TOOL, id_key="ToolId", source="agentkit",
                       operation="worker_state", kind="worker")
        raise DeploymentError(FAILED)
    task = asyncio.create_task(runner.enriched_provision(provision))
    await asyncio.wait_for(entered.wait(), 1)
    task.cancel()
    with pytest.raises(asyncio.CancelledError): await task
    assert runner._trace.get() is None


@pytest.mark.asyncio
async def test_successful_worker_creation_matches_pinned_sdk_requests_and_saved_identity(monkeypatch, installed):
    from veadk.integrations.mpa.managed import worker
    monkeypatch.setattr(worker.uuid, "uuid4", lambda: "fixed-client-token")
    monkeypatch.setattr(worker.asyncio, "sleep", AsyncMock())
    release, audit = AsyncMock(), AsyncMock()
    monkeypatch.setattr(runner, "worker_release_diagnostic", release)
    monkeypatch.setattr(runner, "audit_events", audit)
    calls, reads = [], 0
    async def api(_self, method, request_type, **values):
        nonlocal reads
        calls.append((method, request_type, deepcopy(values)))
        if method == "list_tools": return {"Tools": []}
        if method == "create_tool": return {"ToolId": TOOL}
        assert method == "get_tool"
        reads += 1
        return tool("Creating" if reads == 1 else "Ready")
    monkeypatch.setattr(WorkerCloud, "call", api)
    async def run(client, *, progress):
        progress("worker")
        record = {}
        entry = SimpleNamespace(read=AsyncMock(side_effect=lambda: deepcopy(record)),
                                save=AsyncMock(side_effect=lambda value: record.update(value)))
        result = await ensure_worker(entry, client, Worker(image="example/image:tag"),
                                     account=ACCOUNT, region=REGION, agent_id="test")
        return result, record
    expected = await run(WorkerCloud(cloud()), progress=lambda _: None)
    expected_calls, calls[:], reads = deepcopy(calls), [], 0
    actual = await runner.enriched_provision(run, service.WorkerCloud(cloud()))
    assert actual == expected and calls == expected_calls
    assert actual[0] == TOOL and actual[1]["worker_id"] == TOOL
    release.assert_not_called()
    audit.assert_not_called()


@pytest.mark.asyncio
@pytest.mark.parametrize("kind", ["network", "gateway", "database"])
async def test_successful_adapter_preserves_create_bodies_reads_and_results(monkeypatch, installed, kind):
    class Source:
        account, region, provider = ACCOUNT, REGION, "volcengine"
        def _credentials(self): return None
    source, calls = Source(), []
    responses = {
        "create_vpc": {"vpc_id": "vpc-test"},
        "create_subnet": {"subnet_id": "subnet-test"},
        "describe_vpc_attributes": {"vpc_id": "vpc-test", "status": "Available", "account_id": ACCOUNT},
        "describe_subnet_attributes": {"subnet_id": "subnet-test", "status": "Available", "account_id": ACCOUNT},
        "CreateGateway": {"Id": "gateway-test"},
        "GetGateway": {"Gateway": {"Id": "gateway-test", "Region": REGION, "Status": "RUNNING"}},
        "CreateIMChannelGateway": {},
        "GetIMChannelGatewayStatus": {"Status": "RUNNING", "IMGatewayPrivateEndpoint": "https://example.invalid", "RuntimeServiceId": "im-test"},
        "create_workspace": {"workspace_id": "workspace-test"},
        "describe_workspace_detail": {"workspace": {"workspace_id": "workspace-test", "workspace_status": "Running"}},
        "describe_branches": {"branches": [{"branch_id": "branch-test", "branch_name": "main", "branch_status": "Running"}]},
        "describe_computes": {"computes": [{"compute_id": "compute-test", "compute_role": "primary", "compute_status": "Running"}]},
    }
    async def api(_self, method, request, **params):
        calls.append(json.dumps([method, request, params], sort_keys=True, default=lambda v: v.to_dict()))
        return deepcopy(responses[method])
    if kind == "network":
        baseline_class, adapted_class = network_cloud.NetworkCloud, runtime.NetworkCloud
        options = dict(region=REGION, credentials=source._credentials)
    elif kind == "gateway":
        baseline_class, adapted_class = gateway_cloud.GatewayCloud, service.GatewayCloud
        options = dict(runtime_cloud=source)
    else:
        baseline_class, adapted_class = pg_cloud.PGCloud.__bases__[0], pg_cloud.PGCloud
        options = dict(region=REGION, credentials=source._credentials)
    monkeypatch.setattr(baseline_class, "call", api)
    release, audit = AsyncMock(), AsyncMock()
    monkeypatch.setattr(runner, "worker_release_diagnostic", release)
    monkeypatch.setattr(runner, "audit_events", audit)
    async def run(client, *, progress):
        progress(kind)
        if kind == "network":
            return [await client.create_vpc({"vpc_name": "test", "cidr_block": "172.20.0.0/16"}),
                    await client.vpc("vpc-test"),
                    await client.create_subnet({"vpc_id": "vpc-test", "subnet_name": "test"}),
                    await client.subnet("subnet-test")]
        if kind == "gateway":
            return [await client.create_gateway("test", "vpc-test", ["subnet-one", "subnet-two"]),
                    await client.get_gateway("gateway-test"),
                    await client.create_im_gateway_service(gateway_id="gateway-test"),
                    vars(await client.get_im_gateway_service_status(gateway_id="gateway-test"))]
        return [await client.create("test", "default", {"owner": "test"}),
                await client.detail("workspace-test"),
                await client.call("describe_branches", "DescribeBranchesRequest", workspace_id="workspace-test"),
                await client.call("describe_computes", "DescribeComputesRequest", workspace_id="workspace-test", branch_id="branch-test")]
    expected = await run(baseline_class(**options), progress=lambda _: None)
    expected_calls, calls[:] = list(calls), []
    actual = await runner.enriched_provision(run, adapted_class(**options))
    assert actual == expected and calls == expected_calls
    release.assert_not_called()
    audit.assert_not_called()


@pytest.mark.asyncio
async def test_openviking_terminal_failure_uses_exact_resource_status(monkeypatch, installed):
    from veadk.integrations.mpa.managed.openviking import OpenVikingCloud, OpenVikingError
    row = {"ResourceID": "ov-test", "Status": "FAILED", "StatusMessage":
           {"Code": "ModelProvisioningFailed", "RequestId": REQUEST, "Message": "PRIVATE"}}
    monkeypatch.setattr(OpenVikingCloud, "get", AsyncMock(return_value=row))
    source = cloud()
    async def provision(*, progress):
        progress("openviking")
        await service.OpenVikingCloud(source).get("ov-test")
        raise OpenVikingError("openvikingResourceFailed", category="resource_failed")
    with pytest.raises(OpenVikingError) as failure: await runner.enriched_provision(provision)
    detail = report_failure("provision", failure.value)
    assert detail["errorCode"] == "ModelProvisioningFailed" and detail["operation"] == "get_openviking"
    assert "PRIVATE" not in json.dumps(detail)


@pytest.mark.asyncio
async def test_parallel_creations_cannot_share_failure_observations(monkeypatch):
    monkeypatch.setattr(runner, "audit_events", AsyncMock())
    async def provision(resource_id, *, progress):
        progress("worker")
        row = {"ToolId": resource_id, "Status": "Failed", "ErrorCode": "ErrorFor" + resource_id}
        runner.observe(cloud(), row, resource_id=resource_id, id_key="ToolId", source="agentkit",
                       operation="worker_state", kind="worker")
        await asyncio.sleep(.01)
        raise DeploymentError(FAILED)
    failures = await asyncio.gather(*(runner.enriched_provision(provision, ident) for ident in ("One", "Two")),
                                    return_exceptions=True)
    assert [report_failure("provision", e)["errorCode"] for e in failures] == ["ErrorForOne", "ErrorForTwo"]
    runner.audit_events.assert_not_called()

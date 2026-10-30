"""Studio creation runner with bounded, read-only resource failure diagnostics.

The pinned orchestration and its exception/retry semantics stay intact. Only a
failed readiness check gets enriched; raw audit payloads never enter the journal.
"""

from __future__ import annotations

import asyncio
from contextvars import ContextVar
from dataclasses import dataclass
from datetime import datetime
import json
import logging
import re
import time

from veadk.integrations.mpa.managed.database import DeploymentError
from veadk.integrations.mpa.managed.diagnostics import (
    exception_details,
    safe_error_code,
    safe_request_id,
    with_diagnostic,
)
from veadk.integrations.mpa.managed.runtime import AgentRuntimeDeployer

FAILED = "Runtime deployment failed; inspect its logs and retry using the registered ID"
LOOKUP_TIMEOUT = 12
DEADLINE_RESERVE = 0.25
MAX_PAGES = 8
MAX_JSON = 65536


def object_value(value):
    if isinstance(value, str) and len(value) <= MAX_JSON:
        try:
            value = json.loads(value)
        except (ValueError, RecursionError):
            return {}
    return value if isinstance(value, dict) else {}


def timestamp(value):
    if not isinstance(value, str):
        return None
    try:
        parsed = datetime.fromisoformat(value)
        return parsed.timestamp() if parsed.tzinfo is not None else None
    except (ValueError, OverflowError):
        return None


def diagnostic(code, request_id="", operation="wait_runtime"):
    code = safe_error_code(code)
    if not code:
        return None
    return {
        "operation": operation, "category": "resource_failed",
        "attempt": 1, "outcome": "failed", "errorCode": code,
        **({"requestId": safe_request_id(request_id)} if safe_request_id(request_id) else {}),
    }


def runtime_diagnostic(runtime):
    """Accept structured identifiers, not arbitrary provider message text."""
    message = runtime.get("StatusMessage")
    value = object_value(message)
    metadata = object_value(value.get("ResponseMetadata"))
    error = object_value(metadata.get("Error") or value.get("Error"))
    code = error.get("Code") or value.get("ErrorCode") or value.get("Code")
    if not code and isinstance(message, str) and len(message) <= MAX_JSON:
        # Some providers prefix a free-text explanation with their dotted code.
        match = re.match(r"^([A-Z][A-Za-z0-9]*(?:[._][A-Za-z0-9]+)+)(?=[:\s]|$)", message)
        code = match.group(1) if match else ""
    return diagnostic(code, metadata.get("RequestId") or value.get("RequestId"))


def resource_diagnostic(row, operation):
    """Read only known structured failure fields; never copy logs or messages."""
    code = row.get("ErrorCode") or row.get("error_code")
    request = row.get("RequestId") or row.get("request_id")
    direct = diagnostic(code, request, operation)
    if direct:
        return direct
    for key in ("StatusMessage", "status_message", "Error", "error", "FailureReason", "failure_reason"):
        value = row.get(key)
        if isinstance(value, dict):
            detail = diagnostic(value.get("Code") or value.get("code"),
                                value.get("RequestId") or value.get("request_id"), operation)
            if detail:
                return detail
        detail = runtime_diagnostic({"StatusMessage": value})
        if detail:
            return {**detail, "operation": operation}
    return None


def failure_window(runtime, started, failed):
    created, updated = timestamp(runtime.get("CreatedAt")), timestamp(runtime.get("UpdatedAt"))
    # Resume of a failed creation uses its recorded failure time. A new update
    # must not accidentally select the old creation's audit records.
    end = min(failed, updated) if updated is not None else failed
    start = max(created if created is not None else started - 15, end - 900)
    return int(start) - 1, int(end) + 2


def audit_diagnostic(trails, *, runtime_id, account, region, start, end,
                     resource_keys=("RuntimeId",), source="agentkit", operation="wait_runtime",
                     qualifiers=None):
    """Resolve exact resource evidence, or an isolated AgentKit dependency failure.

    A shared VPC and nearby timestamp alone are insufficient. For the PrivateLink
    fallback require the exact Runtime creation/update receipt, a complete audit
    window with no competing AgentKit writes, and one dependency request identity.
    """
    noun = {
        ("RuntimeId",): "Runtime", ("ToolId",): "Tool", ("VpcId",): "Vpc",
        ("SubnetId",): "Subnet", ("Id",): "Gateway", ("GatewayId",): "IMChannelGateway",
        ("ResourceID",): "OpenVikingCollection", ("WorkspaceId",):
            "Compute" if qualifiers and "ComputeId" in qualifiers
            else "Branch" if qualifiers and "BranchId" in qualifiers else "Workspace",
    }.get(resource_keys)
    actions = {verb + noun for verb in ("Create", "Update", "Modify")} if noun else set()
    events = []
    for trail in trails:
        detail = object_value(trail.get("event_detail", trail.get("EventDetail")))
        identity = object_value(detail.get("UserIdentity"))
        event_time = timestamp(detail.get("EventTime"))
        if (str(identity.get("AccountID", "")) != account or detail.get("Region") != region
                or event_time is None or not start <= event_time <= end):
            continue
        params = object_value(detail.get("RequestParameters"))
        response = object_value(detail.get("ResponseElements"))
        result = object_value(response.get("Result"))
        resources = detail.get("RelatedResources") or detail.get("Resources") or []
        exact = detail.get("EventSource") == source and any(
            params.get(key) == runtime_id or result.get(key) == runtime_id for key in resource_keys)
        if isinstance(resources, list):
            exact = exact or (detail.get("EventSource") == source and any(
                isinstance(r, dict) and r.get("ResourceID") == runtime_id for r in resources))
        if qualifiers:
            exact = exact and all(params.get(key) == value for key, value in qualifiers.items())
        # A subnet/endpoint write mentioning a shared VPC is not a VPC failure;
        # likewise, unrelated Workspace or gateway subresource writes are excluded.
        exact = exact and detail.get("EventName") in actions
        events.append({
            "source": detail.get("EventSource"), "name": detail.get("EventName"),
            "time": event_time, "exact": exact, "params": params,
            "identity": identity, "code": safe_error_code(detail.get("ErrorCode")),
            "request": safe_request_id(detail.get("RequestID")), "result": result,
        })
    direct = [e for e in events if e["exact"] and e["code"]]
    if direct:
        latest = max(direct, key=lambda e: e["time"])
        return diagnostic(latest["code"], latest["request"], operation)
    if source != "agentkit" or qualifiers:
        return None  # Other services require exact resource evidence, never time alone.
    writes = [e for e in events if e["source"] == "agentkit"
              and isinstance(e["name"], str) and re.match(r"^(Create|Update|Delete|Deploy|Release)", e["name"])]
    if (len(writes) != 1 or not writes[0]["exact"] or writes[0]["code"]
            or writes[0]["name"] not in ({"CreateTool", "UpdateTool"} if resource_keys == ("ToolId",)
                                        else {"CreateRuntime", "UpdateRuntime"})):
        return None
    anchor = writes[0]
    prefix = "agentkit-builtin-tools-" if resource_keys == ("ToolId",) else "agentkit-runtime-"
    # The control plane binds FaaS functions by this stable, exact resource name.
    # Resolve successful CreateFunction receipts before considering release errors.
    functions = {e["result"].get("Id") for e in events
                 if e["source"] == "vefaas" and e["name"] == "CreateFunction"
                 and e["params"].get("Name") == prefix + runtime_id and not e["code"]
                 and e["time"] >= anchor["time"] and isinstance(e["result"].get("Id"), str)
                 and e["result"].get("Id")}
    function_errors = [e for e in events if e["source"] == "vefaas" and e["code"]
                       and e["time"] >= anchor["time"]
                       and str(e["identity"].get("PrincipalID", "")).startswith("ServiceRoleForAgentKit/")
                       and ((e["name"] == "CreateFunction" and e["params"].get("Name") == prefix + runtime_id)
                            or (len(functions) == 1 and e["name"] == "ReleaseFunction"
                                and e["params"].get("FunctionId") in functions))]
    function_errors = [e for e in function_errors if not any(
        later["source"] == e["source"] and later["name"] == e["name"] and not later["code"]
        and later["time"] >= e["time"]
        and all(later["params"].get(key) == e["params"].get(key)
                for key in (("Name",) if e["name"] == "CreateFunction" else ("FunctionId",)))
        for later in events)]
    if function_errors:
        latest = max(function_errors, key=lambda e: e["time"])
        return diagnostic(latest["code"], latest["request"], operation)
    dependencies = [e for e in events
                    if e["source"] == "privatelink" and e["name"] == "CreateVpcEndpoint"
                    and e["code"] and e["time"] >= anchor["time"]
                    and str(e["identity"].get("PrincipalID", "")).startswith("ServiceRoleForAgentKit/")]
    dependencies = [e for e in dependencies if not any(
        later["source"] == "privatelink" and later["name"] == "CreateVpcEndpoint"
        and not later["code"] and later["time"] >= e["time"]
        and all(later["params"].get(key) == e["params"].get(key)
                for key in ("VpcId", "ServiceId", "ClientToken")) for later in events)]
    identities = {(e["params"].get("VpcId"), e["params"].get("ServiceId"), e["params"].get("ClientToken"), e["code"])
                  for e in dependencies}
    if len(identities) != 1 or not all(next(iter(identities))):
        return None
    latest = max(dependencies, key=lambda e: e["time"])
    return diagnostic(latest["code"], latest["request"], operation)


async def audit_events(cloud, start, end):
    """Read a complete, bounded audit window with this deployment's credentials."""
    def page(token):
        import volcenginesdkcloudtrail as sdk
        import volcenginesdkcore
        from veadk.integrations.mpa.managed.provider import managed_host

        credential = cloud._credentials()  # also revalidates the deployment account
        config = volcenginesdkcore.Configuration()
        config.ak, config.sk = credential.access_key_id, credential.secret_access_key
        config.session_token, config.region = credential.session_token, cloud.region
        config.host = managed_host("cloud_trail", cloud.region, cloud.provider)
        config.connect_timeout, config.read_timeout, config.auto_retry = 2, 4, False
        api_client = volcenginesdkcore.ApiClient(config)
        client = sdk.CLOUDTRAILApi(api_client)
        request = sdk.LookupEventsRequest(
            start_time=start, end_time=end, max_results=50, next_token=token,
            lookup_conditions=[sdk.LookupConditionForLookupEventsInput(
                lookup_condition_key="ReadOnly", lookup_condition_value="false")],
        )
        try:
            return client.lookup_events(request, _request_timeout=(2, 4)).to_dict()
        finally:
            api_client.rest_client.pool_manager.clear()

    rows, seen, token = [], set(), ""
    for _ in range(MAX_PAGES):
        result = await asyncio.to_thread(page, token)
        trails = result.get("trails")
        if not isinstance(trails, list) or len(trails) > 50:
            return None
        rows.extend(trails)
        token = result.get("next_token")
        if not token:
            return rows
        if not isinstance(token, str) or len(token) > 8192 or token in seen:
            return None
        seen.add(token)
    return None  # truncated windows cannot establish that a deployment is isolated


async def diagnose_runtime(cloud, runtime_id, started):
    runtime = await cloud.get(runtime_id)
    if runtime.get("RuntimeId") != runtime_id or runtime.get("Status") not in {"Error", "Failed"}:
        return None
    detail = runtime_diagnostic(runtime)
    if detail:
        return detail
    if cloud.provider != "volcengine":
        return None
    start, end = failure_window(runtime, started, time.time())
    for attempt in range(2):
        trails = await audit_events(cloud, start, end)
        if trails is not None:
            detail = audit_diagnostic(trails, runtime_id=runtime_id, account=str(cloud.account),
                                      region=cloud.region, start=start, end=end)
            if detail:
                return detail
        if attempt == 0:
            await asyncio.sleep(1)  # allow audit delivery; never repeat a cloud write
    return None


class DiagnosticRuntimeDeployer(AgentRuntimeDeployer):
    async def wait_platform(self, runtime_id, expected=None):
        started = time.time()
        try:
            return await super().wait_platform(runtime_id, expected)
        except DeploymentError as error:
            budget = lookup_budget()
            if str(error) == FAILED and budget > 0:
                try:
                    detail = await asyncio.wait_for(diagnose_runtime(self.cloud, runtime_id, started), budget)
                    if detail:
                        with_diagnostic(error, {**exception_details(error), **detail})
                except Exception:
                    # Missing audit permission, delivery lag or lookup failures
                    # must preserve the original deployment error and task state.
                    pass
            raise


@dataclass
class Observation:
    cloud: object
    resource_id: str
    keys: tuple[str, ...]
    source: str
    operation: str
    kind: str
    # Retain only safe identifiers/timestamps, never raw resource responses.
    row: dict
    detail: dict | None
    qualifiers: dict | None = None


@dataclass
class CreationTrace:
    stage: str = ""
    started: float = 0
    observation: Observation | None = None
    deadline: float | None = None


_trace: ContextVar[CreationTrace | None] = ContextVar("desktop_creation_trace", default=None)


def lookup_budget():
    trace = _trace.get()
    if trace is None or trace.deadline is None:
        return LOOKUP_TIMEOUT
    # The pinned runner wraps provisioning in wait_for(profile timeout). Finish
    # diagnostics before that timer can replace an existing resource failure.
    return max(0, min(LOOKUP_TIMEOUT, trace.deadline - time.monotonic() - DEADLINE_RESERVE))


def observe(cloud, row, *, resource_id, id_key, source, operation, kind,
            status_key="Status", qualifiers=None):
    trace = _trace.get()
    if trace is None:
        return
    trace.observation = None  # A later successful read supersedes a recovered failure.
    if (not isinstance(row, dict) or not isinstance(resource_id, str) or not resource_id
            or row.get(id_key) != resource_id):
        return
    if str(row.get(status_key, "")).lower() not in {"error", "failed", "createfailed"}:
        return
    # The orchestration still validates ownership/configuration before failing.
    if row.get("account_id") is not None and str(row["account_id"]) != str(cloud.account):
        return
    if row.get("region_id") is not None and row["region_id"] != cloud.region:
        return
    times = {}
    for target, aliases in {
        "CreatedAt": ("CreatedAt", "creation_time", "create_time"),
        "UpdatedAt": ("UpdatedAt", "status_changed_time", "update_time"),
    }.items():
        for key in aliases:
            if timestamp(row.get(key)) is not None:
                times[target] = row[key]
                break
    trace.observation = Observation(cloud, resource_id, (id_key,), source, operation, kind,
                                    times, resource_diagnostic(row, operation), qualifiers)


def credential_source(adapter):
    """Adapters use the Runtime client's account-revalidating bound credentials."""
    return getattr(adapter.credentials, "__self__", None)


async def worker_release_diagnostic(cloud, tool_id, start, end):
    """Inspect the exact Worker function's failed release, without reading logs."""
    if cloud.provider != "volcengine":
        return None

    def read():
        import volcenginesdkcore as core
        import volcenginesdkvefaas as sdk
        from veadk.integrations.mpa.managed.provider import managed_host

        credential = cloud._credentials()
        config = core.Configuration()
        config.ak, config.sk = credential.access_key_id, credential.secret_access_key
        config.session_token, config.region = credential.session_token, cloud.region
        config.host = managed_host("vefaas", cloud.region, cloud.provider)
        config.connect_timeout, config.read_timeout, config.auto_retry = 2, 3, False
        client = core.ApiClient(config)
        try:
            api = sdk.VEFAASApi(client)
            name = "agentkit-builtin-tools-" + tool_id
            result = api.list_functions(sdk.ListFunctionsRequest(
                filters=[sdk.FilterForListFunctionsInput(name="Name", values=[name])],
                page_number=1, page_size=2), _request_timeout=(2, 3)).to_dict()
            items = result.get("items")
            if (result.get("total") != 1 or not isinstance(items, list) or len(items) != 1
                    or items[0].get("name") != name or not items[0].get("id")):
                return None
            function_id = items[0]["id"]
            release = api.get_release_status(sdk.GetReleaseStatusRequest(function_id=function_id),
                                            _request_timeout=(2, 3)).to_dict()
            when = timestamp(release.get("start_time"))
            if (release.get("function_id") != function_id or release.get("status") != "failed"
                    or when is None or not start <= when <= end):
                return None
            return resource_diagnostic(release, "worker_state")
        finally:
            client.rest_client.pool_manager.clear()

    return await asyncio.to_thread(read)


async def diagnose_observation(observation, started):
    if observation.detail:
        return observation.detail
    start, end = failure_window(observation.row, started, time.time())
    if observation.kind == "worker":
        try:
            # Reserve time for audit lookup even when FaaS permissions are missing.
            detail = await asyncio.wait_for(worker_release_diagnostic(
                observation.cloud, observation.resource_id, start, end), 5)
            if detail:
                return detail
        except Exception:
            pass
    if observation.cloud.provider != "volcengine":
        return None
    for attempt in range(2):
        trails = await audit_events(observation.cloud, start, end)
        if trails is not None:
            detail = audit_diagnostic(
                trails, runtime_id=observation.resource_id, account=str(observation.cloud.account),
                region=observation.cloud.region, start=start, end=end,
                resource_keys=observation.keys, source=observation.source,
                operation=observation.operation, qualifiers=observation.qualifiers)
            if detail:
                return detail
        if attempt == 0:
            await asyncio.sleep(1)
    return None


def matches_failure(observation, error):
    """Only enrich the original terminal-state failure, never identity/SQL/API errors."""
    message = str(error)
    if observation.kind == "worker":
        return message == "Worker is not ready; inspect it and retry the same agent"
    if observation.kind in {"vpc", "subnet"}:
        return message == f"Registered {observation.kind} is unavailable; repair the resource before retrying"
    if observation.kind == "gateway":
        return message == "Shared gateway is unavailable; repair the existing resource"
    if observation.kind == "im_gateway":
        return message == "IM Gateway failed; inspect the registered service"
    if observation.kind == "postgres":
        return message in {
            "PostgreSQL Workspace creation failed; correct AIDAP permissions or configuration before retrying",
            "PostgreSQL resource has an unsupported or failed state; check AIDAP console",
        }
    if observation.kind == "openviking":
        return getattr(error, "key", None) == "openvikingResourceFailed"
    return False


async def enriched_provision(provision, *args, **kwargs):
    trace = CreationTrace(started=time.time())
    profile = args[0] if args else kwargs.get("profile")
    timeout = getattr(getattr(profile, "managed", None), "timeout_seconds", None)
    if type(timeout) in (int, float) and 0 < timeout < float("inf"):
        trace.deadline = time.monotonic() + timeout
    token = _trace.set(trace)
    progress = kwargs.get("progress", lambda stage: None)

    def step(stage):
        if stage != trace.stage:
            trace.stage, trace.started, trace.observation = stage, time.time(), None
        progress(stage)

    try:
        return await provision(*args, **{**kwargs, "progress": step})
    except Exception as error:
        # Some pinned database boundaries suppress __cause__. Preserve SQLSTATE
        # through __context__ without forwarding connection strings or SQL text.
        from sqlalchemy.exc import DBAPIError
        original = getattr(error, "_mpa_creation_diagnostic", None) or {}
        cause, seen = error, set()
        while (not original.get("errorCode") and cause is not None
               and id(cause) not in seen and len(seen) < 8):
            seen.add(id(cause))
            state = getattr(getattr(cause, "orig", None), "sqlstate", None)
            if isinstance(cause, DBAPIError) and isinstance(state, str) and re.fullmatch(r"[0-9A-Z]{5}", state):
                detail = diagnostic("PostgreSQL." + state, operation="prepare_database")
                detail["category"] = "permission" if state.startswith("28") or state == "42501" else "deployment_error"
                with_diagnostic(error, {**exception_details(cause), **detail})
                break
            cause = cause.__cause__ or cause.__context__
        observation = trace.observation
        budget = lookup_budget()
        if observation is not None and matches_failure(observation, error) and budget > 0:
            try:
                detail = await asyncio.wait_for(diagnose_observation(observation, trace.started), budget)
                if detail:
                    with_diagnostic(error, {**exception_details(error), **detail})
            except Exception:
                pass  # Diagnostics cannot alter the resource's original failure/retry contract.
        raise
    finally:
        _trace.reset(token)


def install_diagnostics():
    """Adapt reads in the child process; keep the pinned orchestration unchanged."""
    from veadk.integrations.mpa.managed import gateway_cloud, network_cloud, openviking, pg_cloud, runner, runtime, service
    from veadk.integrations.mpa.managed.worker import WorkerCloud

    class DiagnosticWorkerCloud(WorkerCloud):
        async def get(self, tool_id):
            row = await super().get(tool_id)
            observe(self.runtime, row, resource_id=tool_id, id_key="ToolId", source="agentkit",
                    operation="worker_state", kind="worker")
            return row

    class DiagnosticNetworkCloud(network_cloud.NetworkCloud):
        async def call(self, method, request_type, **params):
            row = await super().call(method, request_type, **params)
            source = credential_source(self)
            if source is not None:
                for kind in ("vpc", "subnet"):
                    if method == f"describe_{kind}_attributes":
                        # SDK responses use snake_case; audit requests use PascalCase.
                        resource_id = params.get(kind + "_id")
                        normalized = {**row, kind.title() + "Id": row.get(kind + "_id")}
                        observe(source, normalized, resource_id=resource_id, id_key=kind.title() + "Id",
                                source="vpc", operation=method, kind=kind, status_key="status")
            return row

    class DiagnosticGatewayCloud(gateway_cloud.GatewayCloud):
        async def call(self, action, body):
            result = await super().call(action, body)
            if action == "GetGateway":
                row = result.get("Gateway") or {}
                if row.get("Region") == self.runtime.region:
                    observe(self.runtime, row, resource_id=body.get("Id"), id_key="Id", source="apig",
                            operation=action, kind="gateway")
            elif action == "GetIMChannelGatewayStatus":
                observe(self.runtime, {**result, "GatewayId": body.get("GatewayId")},
                        resource_id=body.get("GatewayId"), id_key="GatewayId", source="apig",
                        operation=action, kind="im_gateway")
            return result

    class DiagnosticPGCloud(pg_cloud.PGCloud):
        async def call(self, method, request_type, **params):
            result = await super().call(method, request_type, **params)
            source = credential_source(self)
            if source is None:
                return result
            row, qualifiers = None, None
            if method == "describe_workspace_detail":
                row = result.get("workspace")
            elif method in {"describe_branches", "describe_computes"}:
                try:
                    if method == "describe_branches":
                        row = pg_cloud.one(result.get("branches") or [], "branch_name", "main")
                        qualifiers = {"BranchId": row["branch_id"]}
                        row = {**row, "workspace_status": row.get("branch_status")}
                    else:
                        active = [c for c in result.get("computes") or [] if not c.get("disabled")]
                        primary = [c for c in active if str(c.get("compute_role", "")).lower()
                                   in {"primary", "readwrite", "read_write"}]
                        row = pg_cloud.one(primary or active, "compute_id")
                        qualifiers = {"BranchId": params["branch_id"], "ComputeId": row["compute_id"]}
                        row = {**row, "workspace_status": row.get("compute_status")}
                    row = {**row, "workspace_id": params["workspace_id"]}
                except (DeploymentError, KeyError, TypeError):
                    row = None
            if row is not None:
                observe(source, {**row, "WorkspaceId": row.get("workspace_id")},
                        resource_id=params.get("workspace_id"), id_key="WorkspaceId", source="aidap",
                        operation=pg_cloud.METHOD_OPERATIONS[method], kind="postgres",
                        status_key="workspace_status", qualifiers=qualifiers)
            return result

    class DiagnosticOpenVikingCloud(openviking.OpenVikingCloud):
        async def get(self, ident):
            row = await super().get(ident)
            observe(self.source, row, resource_id=ident, id_key="ResourceID", source="vikingdb",
                    operation="get_openviking", kind="openviking")
            return row

    original = service.provision

    async def provision(*args, **kwargs):
        return await enriched_provision(original, *args, **kwargs)

    service.WorkerCloud = DiagnosticWorkerCloud
    runtime.NetworkCloud = DiagnosticNetworkCloud
    service.GatewayCloud = DiagnosticGatewayCloud
    pg_cloud.PGCloud = DiagnosticPGCloud
    service.OpenVikingCloud = DiagnosticOpenVikingCloud
    service.AgentRuntimeDeployer = DiagnosticRuntimeDeployer
    runner.provision = provision


def main():
    from veadk.integrations.mpa.managed import runner

    logging.disable(logging.CRITICAL)
    install_diagnostics()
    return runner.main()


if __name__ == "__main__":
    raise SystemExit(main())

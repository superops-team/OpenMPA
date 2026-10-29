"""Private stdin/stdout bridge to Studio's managed MPA lifecycle.

Only validated operation names are accepted. The engine has no listening socket.
The Go host supplies one verified cloud identity and a private state directory.
"""

from __future__ import annotations

import asyncio
import contextlib
import copy
from contextvars import ContextVar
import hashlib
import json
import logging
import os
import re
import sqlite3
import sys
import time
import traceback
from pathlib import Path
from typing import Any
from uuid import UUID
from urllib.parse import urlsplit

import httpx
from fastapi import FastAPI
from frontend.server.mpa.runtime_client import MpaRuntimeClient
from frontend.server.mpa_creation import mount_mpa_creation_routes
from veadk.integrations.mpa.managed.credentials import Credentials
from veadk.integrations.mpa.managed.deletion import owned_resource_cleanup
from veadk.integrations.mpa.managed.diagnostics import classify_error, exception_details, failure_identifiers
from veadk.integrations.mpa.managed.runtime import RuntimeCloud, env_map
from veadk.integrations.mpa.managed.tasks import CreationTasks
from veadk.integrations.mpa.managed.tasks import owner_key
from veadk.integrations.mpa.managed.tasks import STAGES

REGIONS = {"cn-beijing", "cn-shanghai"}
ACTIVE = {"running", "queued", "pending", "starting", "cancelling", "stopping", "pausing", "paused", "resuming"}
IDENTIFIER = re.compile(r"[A-Za-z0-9_-]{1,128}\Z")
_delete_phase = ContextVar("desktop_delete_phase", default=("checking", "preview_delete"))
PREVIEW_TIMEOUT = 60


class LifecycleError(Exception):
    def __init__(self, code: str, status: int = 409, diagnostic=None):
        self.code, self.status = code, status
        self.diagnostic = diagnostic


def deletion_diagnostic(error, stage, operation):
    cause = error
    saved = None
    for _ in range(8):
        if cause is None: break
        saved = getattr(cause, "_openmpa_diagnostic", None)
        if saved: break
        cause = cause.__cause__
    if saved:
        return {**saved, **({"category": "timeout"} if isinstance(error, TimeoutError) else {})}
    details = exception_details(error)
    code, request_id = failure_identifiers(error)
    if code: details["errorCode"] = code
    if request_id: details["requestId"] = request_id
    status = getattr(error, "status_code", None)
    if type(status) is int and 100 <= status <= 599: details["httpStatus"] = status
    category = classify_error(error)
    if details.get("httpStatus") in (401, 403): category = "permission"
    elif details.get("httpStatus") == 404: category = "not_found"
    elif details.get("httpStatus") == 429: category = "throttled"
    elif details.get("httpStatus") in (500, 502, 503, 504): category = "unavailable"
    diagnostic = dict(stage=stage, operation=operation, category=category,
                      attempt=1, outcome="failed", **details)
    reason = getattr(error, "reason", "")
    if reason in {"runtime_binding_conflict", "active_image_update", "active_deployment",
                  "agent_binding_conflict", "resource_binding_conflict", "ownership_conflict"}:
        diagnostic.update(stage=getattr(error, "stage", stage), reason=reason, category="ownership")
    frames = traceback.extract_tb(error.__traceback__)
    if frames:
        frame = frames[-1]
        source = f"{Path(frame.filename).name}:{frame.lineno}:{frame.name}"
        if re.fullmatch(r"[A-Za-z0-9_.-]+\.py:[0-9]{1,6}:[A-Za-z0-9_<>]{1,80}", source):
            diagnostic["source"] = source
    return diagnostic


@contextlib.contextmanager
def deletion_step(stage, operation):
    _delete_phase.set((stage, operation))
    try:
        yield
    except (Exception, asyncio.CancelledError) as error:
        error.__dict__.setdefault("_openmpa_diagnostic", deletion_diagnostic(error, stage, operation))
        raise


class DiagnosticProxy:
    """Keep the original cleanup implementation, adding only operation context."""
    def __init__(self, target, methods):
        self.target, self.methods = target, methods

    def __getattr__(self, name):
        value = getattr(self.target, name)
        if name not in self.methods:
            return value
        async def traced(*args, **kwargs):
            with deletion_step(*self.methods[name]):
                return await value(*args, **kwargs)
        return traced


class DeletionCloud(RuntimeCloud):
    """One verified identity and directory snapshot per deletion check.

    A new instance is created for each preview and accepted deletion. The
    directory is explicitly invalidated before the destructive cleanup plan.
    """
    def __init__(self, **kwargs):
        super().__init__(cache_identity=True, **kwargs)
        self._directory = None

    async def list_runtimes(self):
        if self._directory is None:
            self._directory = await super().list_runtimes()
        return copy.deepcopy(self._directory)

    def invalidate_directory(self):
        self._directory = None


@contextlib.asynccontextmanager
async def diagnostic_cleanup(**kwargs):
    kwargs["cloud"] = DiagnosticProxy(kwargs["cloud"], {
        "account_id": ("registry", "get_caller_identity"),
        "list_runtimes": ("references", "list_runtimes"),
        "get_skill_space": ("skill_space", "get_skill_space"),
    })
    async with owned_resource_cleanup(**kwargs) as cleanup:
        cleanup.worker = DiagnosticProxy(cleanup.worker, {
            "get": ("worker_template", "get_worker"),
            "delete": ("worker_template", "delete_worker"),
        })
        yield cleanup


class DesktopCreationTasks(CreationTasks):
    """Persist observed stages in the same transaction as Studio's supervisor.

    SQL triggers also cover the supervisor's writes and fast stages between UI
    polls. Retrying the original task starts a fresh stage journal.
    """
    def __init__(self, path):
        super().__init__(path)
        with self.db() as db:
            db.executescript("""
                CREATE TABLE IF NOT EXISTS desktop_creation_stages (
                    id INTEGER PRIMARY KEY, task_id TEXT NOT NULL,
                    stage TEXT NOT NULL, UNIQUE(task_id, stage)
                );
                CREATE TRIGGER IF NOT EXISTS desktop_creation_stage_insert
                AFTER INSERT ON tasks BEGIN
                    INSERT OR IGNORE INTO desktop_creation_stages(task_id,stage)
                        VALUES(NEW.id,NEW.stage);
                END;
                CREATE TRIGGER IF NOT EXISTS desktop_creation_stage_update
                AFTER UPDATE OF stage ON tasks BEGIN
                    DELETE FROM desktop_creation_stages
                        WHERE task_id=NEW.id AND NEW.stage='queued';
                    INSERT OR IGNORE INTO desktop_creation_stages(task_id,stage)
                        VALUES(NEW.id,NEW.stage);
                END;
            """)

    def get(self, owner, task_id):
        value = super().get(owner, task_id)
        with self.db() as db:
            # A stage can change between the superclass read and this query.
            # Read status and history from one snapshot so a later stage is
            # never shown as completed beside an older running stage.
            db.execute("BEGIN")
            current = db.execute(
                "SELECT state,stage,result,error FROM tasks WHERE id=? AND owner=?",
                (task_id, owner),
            ).fetchone()
            value.update(state=current["state"], stage=current["stage"],
                         result=json.loads(current["result"]), error=current["error"])
            rows = db.execute(
                "SELECT stage FROM desktop_creation_stages WHERE task_id=? ORDER BY id",
                (task_id,),
            ).fetchall()
        # Old journals have no history. Do not invent completed stages for them.
        if rows:
            value["visitedStages"] = [row[0] for row in rows if row[0] in STAGES]
        return value


class Engine:
    def __init__(self, account: str, owner: str, directory: Path, *, cloud_factory=None,
                 runtime_client=None, cleanup=None, creations=None):
        self.account, self.owner = account, owner
        self.directory = directory
        directory.mkdir(mode=0o700, parents=True, exist_ok=True)
        os.chmod(directory, 0o700)
        self.creations = creations or DesktopCreationTasks(directory / "creations.sqlite3")
        self.app = FastAPI()
        mount_mpa_creation_routes(self.app, owner=lambda _request: owner, service=self.creations)
        self.http = httpx.AsyncClient(transport=httpx.ASGITransport(app=self.app), base_url="http://engine")
        self.cloud_factory = cloud_factory or (lambda region: DeletionCloud(region=region, credential_file=""))
        self.runtime = runtime_client or MpaRuntimeClient()
        self.cleanup = cleanup or diagnostic_cleanup
        self.deleting: dict[str, asyncio.Task] = {}
        self.intent_lock = asyncio.Lock()
        self.dbpath = directory / "deletions.sqlite3"
        with self.db() as db:
            db.execute("CREATE TABLE IF NOT EXISTS deletions (id TEXT PRIMARY KEY, payload TEXT NOT NULL, view TEXT NOT NULL)")
            db.execute("CREATE TABLE IF NOT EXISTS diagnostics (id INTEGER PRIMARY KEY, operation TEXT NOT NULL, created REAL NOT NULL, diagnostic TEXT NOT NULL)")
            for row in db.execute("SELECT id,view FROM deletions").fetchall():
                view = json.loads(row[1])
                if view["state"] == "running":
                    view.update(state="unknown", error="outcomeUnknown")
                    db.execute("UPDATE deletions SET view=? WHERE id=?", (json.dumps(view), row[0]))
        os.chmod(self.dbpath, 0o600)

    def db(self):
        return contextlib.closing(sqlite3.connect(self.dbpath, isolation_level=None))

    async def dispatch(self, operation: str, body: dict) -> Any:
        region = body.get("region", "cn-beijing")
        if region not in REGIONS:
            raise LifecycleError("invalidInput", 400)
        creation = {
            "config": ("GET", f"/web/mpa-creation/config?region={region}"),
            "listCreations": ("GET", f"/web/mpa-creation/tasks?region={region}&limit=50"),
            "checkName": ("POST", "/web/mpa-creation/name-check"),
            "create": ("POST", "/web/mpa-creation/tasks"),
        }
        task_id = body.get("taskId", "")
        if operation in {"readCreation", "cancelCreation"}:
            try:
                UUID(task_id)
            except (ValueError, TypeError):
                raise LifecycleError("invalidInput", 400) from None
            saved = self.creations.get(owner_key(self.owner), task_id)
            if saved.get("region") != region:
                raise LifecycleError("resourceNotVisible", 404)
            creation[operation] = (
                "GET" if operation == "readCreation" else "POST",
                f"/web/mpa-creation/tasks/{task_id}" + ("/cancel" if operation == "cancelCreation" else ""),
            )
        if operation in creation:
            method, path = creation[operation]
            payload = {k: v for k, v in body.items() if k not in {"taskId"}} if operation in {"create", "checkName"} else None
            if operation == "create":
                async with self.intent_lock:
                    if self.deleting:
                        raise LifecycleError("operationBusy")
                    response = await self.http.request(method, path, json=payload)
            else:
                response = await self.http.request(method, path, json=payload)
            if response.status_code >= 400:
                detail = response.json().get("detail", {})
                code = detail.get("code", "") if isinstance(detail, dict) else ""
                raise LifecycleError(code if IDENTIFIER.fullmatch(code) else "creationRequestFailed", response.status_code)
            return response.json()
        if operation == "previewDelete":
            return await self.preview(body)
        if operation == "delete":
            async with self.intent_lock:
                return await self.start_delete(body)
        if operation == "readDeletion":
            return await self.read_delete(body)
        if operation == "listDeletions":
            with self.db() as db:
                return {"tasks": [json.loads(row[0]) for row in db.execute("SELECT view FROM deletions ORDER BY rowid DESC LIMIT 50") if json.loads(row[0])["region"] == region]}
        raise LifecycleError("invalidInput", 400)

    async def target(self, body):
        runtime_id, region = body.get("runtimeId", ""), body.get("region", "")
        if not IDENTIFIER.fullmatch(runtime_id) or region not in REGIONS:
            raise LifecycleError("invalidInput", 400)
        cloud = self.cloud_factory(region)
        with deletion_step("checking", "get_caller_identity"):
            account = await cloud.account_id()
        if account != self.account:
            raise LifecycleError("scopeMismatch", 403)
        with deletion_step("checking", "get_runtime"):
            record = await cloud.get(runtime_id)
        tags = {tag["Key"]: tag.get("Value", "") for tag in record.get("Tags", [])}
        values = env_map(record)
        agent_id = values.get("MPA_AGENT_ID", "")
        if record.get("RuntimeId") != runtime_id or tags.get("veadk:agent-type") != "mpa" or not IDENTIFIER.fullmatch(agent_id):
            raise LifecycleError("resourceNotVisible", 404)
        if tags.get("veadk:mpa-instance-id", agent_id) != agent_id:
            raise LifecycleError("ownershipConflict")
        endpoint = record.get("Endpoint", "")
        for network in record.get("NetworkConfigurations", []):
            if network.get("NetworkType", "").casefold() == "public" and network.get("Endpoint"):
                endpoint = network["Endpoint"]
                break
        parsed = urlsplit(endpoint)
        if parsed.scheme != "https" or not parsed.hostname or parsed.username or parsed.password or parsed.fragment:
            raise LifecycleError("runtimeEndpointUnavailable")
        auth = record.get("AuthorizerConfiguration", {})
        key = auth.get("KeyAuth", {}).get("ApiKey", "")
        if not key:
            raise LifecycleError("runtimeCredentialUnavailable")
        with deletion_step("checking", "list_runtimes"):
            matches = [item for item in await cloud.list_runtimes()
                       if env_map(item).get("MPA_AGENT_ID") == agent_id]
        if len(matches) != 1 or matches[0].get("RuntimeId") != runtime_id:
            raise LifecycleError("ownershipConflict")
        with deletion_step("runtime_sessions", "list_sessions"):
            sessions = await self.runtime.list_sessions(endpoint=endpoint, bearer_token=key, include_a2a=True, user_id=self.owner)
        return cloud, record, agent_id, endpoint, key, sessions

    def preview_value(self, body, record, agent_id, sessions, resources):
        active = sum(session.status.casefold() in ACTIVE for session in sessions)
        blockers = ["active_sessions"] if active else []
        if record.get("Status", "").casefold() not in {"ready", "running"}:
            blockers.append("runtime_not_ready")
        payload = {
            "runtimeId": body["runtimeId"], "region": body["region"], "agentId": agent_id,
            "name": record.get("Name", body["runtimeId"]), "canDelete": not blockers,
            "blockers": blockers, "sessionCounts": {"total": len(sessions), "active": active, "idle": len(sessions) - active},
            "cleanupPlan": [{"stage": "runtime_sessions", "count": len(sessions)}]
                + resources.get("cleanupPlan", []) + [{"stage": "agentkit_runtime", "count": 1}],
            "retainedResources": resources.get("retainedResources", []),
        }
        snapshot = {"record": record, "sessions": [s.model_dump(mode="json") for s in sessions], "preview": payload}
        payload["fingerprint"] = hashlib.sha256(json.dumps(snapshot, sort_keys=True).encode()).hexdigest()
        return payload

    async def preview(self, body):
        token = _delete_phase.set(("checking", "preview_delete"))
        registry_host = ""
        try:
            async with asyncio.timeout(PREVIEW_TIMEOUT):
                cloud, record, agent_id, _, _, sessions = await self.target(body)
                registry_host = urlsplit(env_map(record).get("SHARED_APIG_DATABASE_URL", "")).hostname or ""
                async with contextlib.AsyncExitStack() as stack:
                    with deletion_step("registry", "open_registry"):
                        cleanup = await stack.enter_async_context(self.cleanup(runtime=record, agent_id=agent_id, region=body["region"], cloud=cloud))
                    with deletion_step("managed_resources", "inspect_resources"):
                        resources = await cleanup.preview()
                with deletion_step("checking", "snapshot_delete"):
                    return self.preview_value(body, record, agent_id, sessions, resources)
        except Exception as error:
            diagnostic = deletion_diagnostic(error, *_delete_phase.get())
            if registry_host and len(registry_host) <= 253 and re.fullmatch(r"[A-Za-z0-9.-]+", registry_host):
                diagnostic["resourceHost"] = registry_host
            with self.db() as db:
                db.execute("INSERT INTO diagnostics(operation,created,diagnostic) VALUES(?,?,?)", ("previewDelete", time.time(), json.dumps(diagnostic)))
                db.execute("DELETE FROM diagnostics WHERE id NOT IN (SELECT id FROM diagnostics ORDER BY id DESC LIMIT 100)")
            if isinstance(error, LifecycleError):
                error.diagnostic = diagnostic
                raise
            raise LifecycleError("deletePreviewFailed", diagnostic.get("httpStatus", 503), diagnostic) from None
        finally:
            _delete_phase.reset(token)

    def save_delete(self, operation_id, payload, view):
        with self.db() as db:
            db.execute("INSERT INTO deletions VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET view=excluded.view",
                       (operation_id, json.dumps(payload, sort_keys=True), json.dumps(view)))

    async def start_delete(self, body):
        try:
            UUID(body.get("operationId", ""))
        except (ValueError, TypeError):
            raise LifecycleError("invalidInput", 400) from None
        if set(body) != {"operationId", "runtimeId", "region", "fingerprint", "confirm"} or body["confirm"] is not True:
            raise LifecycleError("invalidInput", 400)
        operation_id = body["operationId"]
        with self.db() as db:
            previous = db.execute("SELECT payload,view FROM deletions WHERE id=?", (operation_id,)).fetchone()
        if previous:
            if previous[0] != json.dumps(body, sort_keys=True):
                raise LifecycleError("conflict")
            return json.loads(previous[1])
        if self.deleting or any(task["state"] in {"running", "cancelling"} for task in self.creations.list(owner_key(self.owner), limit=50)):
            raise LifecycleError("operationBusy")
        preview = await self.preview(body)
        if not preview["canDelete"] or preview["fingerprint"] != body["fingerprint"]:
            raise LifecycleError("staleDeletePreview")
        view = {"operationId": operation_id, "runtimeId": body["runtimeId"], "region": body["region"],
                "name": preview["name"], "state": "running", "stage": "checking", "error": "", "updatedAt": time.time()}
        self.save_delete(operation_id, body, view)
        task = asyncio.create_task(self.run_delete(body, view))
        self.deleting[operation_id] = task
        task.add_done_callback(lambda _task: self.deleting.pop(operation_id, None))
        return dict(view)

    async def run_delete(self, body, view):
        def stage(value):
            _delete_phase.set((value, value))
            view.update(stage=value, updatedAt=time.time())
            self.save_delete(body["operationId"], body, view)
        try:
            async with asyncio.timeout(180):
                await self.delete_resources(body, stage)
            view.update(state="succeeded", stage="complete")
        except asyncio.CancelledError:
            view.update(state="unknown", error="outcomeUnknown")
        except Exception as error:
            category = classify_error(error)
            view.update(state="failed" if isinstance(error, LifecycleError) else "unknown",
                        error=error.code if isinstance(error, LifecycleError) else category if IDENTIFIER.fullmatch(category) else "outcomeUnknown",
                        latestDiagnostic=deletion_diagnostic(error, *_delete_phase.get()))
        finally:
            view["updatedAt"] = time.time()
            self.save_delete(body["operationId"], body, view)

    async def delete_resources(self, body, stage):
        cloud, record, agent_id, endpoint, key, sessions = await self.target(body)
        async with self.cleanup(runtime=record, agent_id=agent_id, region=body["region"], cloud=cloud) as cleanup:
            fresh = self.preview_value(body, record, agent_id, sessions, await cleanup.preview())
            if not fresh["canDelete"] or fresh["fingerprint"] != body["fingerprint"]:
                raise LifecycleError("staleDeletePreview")
            stage("runtime_sessions")
            for session in sessions:
                session_id = session.session_id or session.id
                if not IDENTIFIER.fullmatch(session_id):
                    raise LifecycleError("invalidResponse")
                await self.runtime.delete_session(endpoint=endpoint, session_id=session_id, bearer_token=key, user_id=self.owner)
            stage("managed_resources")
            if isinstance(cloud, DeletionCloud):
                cloud.invalidate_directory()
            await cleanup.delete_resources()
            stage("agentkit_runtime")
            from agentkit.sdk.runtime.types import DeleteRuntimeRequest
            await cloud.call("delete_runtime", DeleteRuntimeRequest(RuntimeId=body["runtimeId"]))
            await cleanup.mark_runtime_deleted()

    async def read_delete(self, body):
        with self.db() as db:
            row = db.execute("SELECT view FROM deletions WHERE id=?", (body.get("operationId", ""),)).fetchone()
        if not row:
            raise LifecycleError("resourceNotVisible", 404)
        view = json.loads(row[0])
        if view["region"] != body.get("region"):
            raise LifecycleError("resourceNotVisible", 404)
        if view["state"] == "unknown" and view["stage"] == "agentkit_runtime":
            # Runtime deletion may have succeeded after the cleanup receipt was
            # lost. Verify the exact provider NotFound code AND a complete list.
            from frontend.server.mpa_deletion_status import runtime_not_found
            cloud = self.cloud_factory(view["region"])
            if await cloud.account_id() != self.account:
                raise LifecycleError("scopeMismatch", 403)
            try:
                await cloud.get(view["runtimeId"])
            except Exception as error:
                if runtime_not_found(error):
                    rows = await cloud.list_runtimes()
                    if not any(item.get("RuntimeId") == view["runtimeId"] for item in rows):
                        view.update(state="succeeded", stage="complete", error="", updatedAt=time.time())
                        with self.db() as db:
                            db.execute("UPDATE deletions SET view=? WHERE id=?", (json.dumps(view), view["operationId"]))
        return view

    async def close(self):
        await self.creations.close()
        for task in list(self.deleting.values()):
            task.cancel()
        await asyncio.gather(*list(self.deleting.values()), return_exceptions=True)
        await self.http.aclose()


async def serve():
    original_stdout = sys.stdout
    sys.stdout = sys.stderr
    logging.disable(logging.CRITICAL)
    initial = json.loads(await asyncio.to_thread(sys.stdin.readline))
    credentials = initial.pop("credentials")
    for name, value in zip(("VOLCENGINE_ACCESS_KEY", "VOLCENGINE_SECRET_KEY", "VOLCENGINE_SESSION_TOKEN"), credentials, strict=True):
        os.environ[name] = value
    engine = Engine(initial["account"], initial["owner"], Path(initial["directory"]))
    pending: set[asyncio.Task] = set()

    def emit(value):
        original_stdout.write(json.dumps(value, ensure_ascii=False) + "\n")
        original_stdout.flush()

    async def handle(request):
        try:
            value = await engine.dispatch(request["operation"], request.get("body", {}))
            emit({"id": request["id"], "ok": True, "value": value})
        except LifecycleError as error:
            emit({"id": request["id"], "ok": False, "error": {"code": error.code, "httpStatus": error.status, "diagnostic": error.diagnostic}})
        except Exception:
            emit({"id": request.get("id", ""), "ok": False, "error": {"code": "lifecycleUnavailable", "httpStatus": 0}})

    emit({"id": "init", "ok": True, "value": {"ready": True}})
    try:
        while line := await asyncio.to_thread(sys.stdin.readline):
            if len(line) > 32768:
                break
            request = json.loads(line)
            if request.get("operation") == "shutdown":
                break
            task = asyncio.create_task(handle(request))
            pending.add(task)
            task.add_done_callback(pending.discard)
    finally:
        await engine.close()
        await asyncio.gather(*pending, return_exceptions=True)


if __name__ == "__main__":
    asyncio.run(serve())

"""Provider contract tests against the pinned Studio routes, without cloud writes."""
import json
from pathlib import Path
import subprocess
import sys
from types import SimpleNamespace
from unittest.mock import AsyncMock, Mock
from uuid import uuid4

import pytest
from engine import Engine, LifecycleError
from veadk.integrations.mpa.managed.config import load_studio_profile


@pytest.mark.parametrize("provider,region,domain", [
    ("volcengine", "cn-beijing", "volces.com"),
    ("volcengine", "cn-shanghai", "volces.com"),
    ("byteplus", "ap-southeast-1", "bytepluses.com"),
])
def test_private_process_selects_provider_credentials_and_studio_defaults(tmp_path, provider, region, domain):
    initialization = {"account": "test-account", "owner": "test-owner", "provider": provider,
                      "directory": str(tmp_path / provider), "credentials": ["test-ak", "test-sk", "test-token"]}
    commands = [initialization, {"id": "config", "operation": "config", "body": {"region": region}},
                {"id": "bad", "operation": "config", "body": {"region": "cn-beijing" if provider == "byteplus" else "ap-southeast-1"}}]
    # Keep stdin alive long enough to read replies, just as the Go host does.
    process = subprocess.Popen([sys.executable, "-I", str(Path(__file__).with_name("engine.py"))],
                               stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                               text=True, env={"HOME": str(tmp_path), "PATH": "/usr/bin:/bin"})
    try:
        process.stdin.write(json.dumps(commands[0]) + "\n"); process.stdin.flush()
        assert json.loads(process.stdout.readline())["ok"]
        replies = {}
        for request in commands[1:]:
            process.stdin.write(json.dumps(request) + "\n"); process.stdin.flush()
            response = json.loads(process.stdout.readline()); replies[response["id"]] = response
        config = replies["config"]["value"]
        assert config["region"] == region
        if region == "cn-shanghai":
            # Studio currently has no managed creation template for Shanghai.
            # Preserve that explicit response; never borrow Beijing defaults.
            assert not config["configured"]
            assert config["error"] == "No managed configuration for the selected region"
            return
        assert config["configured"]
        assert f".cr.{domain}/mpa/" in config["runtimeImage"]
        assert f".cr.{domain}/mpa/" in config["workerImage"]
        assert "test-sk" not in json.dumps(replies)
        assert not replies["bad"]["ok"] and replies["bad"]["error"]["code"] == "invalidInput"
    finally:
        process.stdin.write('{"operation":"shutdown"}\n'); process.stdin.flush()
        process.communicate(timeout=10)
    assert process.returncode == 0


@pytest.mark.asyncio
@pytest.mark.parametrize("provider,region", [("volcengine", "cn-beijing"), ("byteplus", "ap-southeast-1")])
async def test_creation_route_keeps_provider_in_name_scope_and_runner_payload(tmp_path, monkeypatch, provider, region):
    import frontend.server.mpa_creation as routes
    prefix = "BYTEPLUS" if provider == "byteplus" else "VOLCENGINE"
    monkeypatch.setenv(prefix + "_ACCESS_KEY", "test-ak")
    monkeypatch.setenv(prefix + "_SECRET_KEY", "test-sk")
    check = AsyncMock(return_value="test-account")
    monkeypatch.setattr(routes, "check_creation_name", check)
    tasks = SimpleNamespace(name_scope_for_request=Mock(return_value=None), check_name=Mock(),
                            start=AsyncMock(return_value={"taskId": "test-task"}), close=AsyncMock())
    engine = Engine("test-account", "test-owner", tmp_path, provider=provider, creations=tasks)
    try:
        request = {"requestId": str(uuid4()), "name": "my-mpa", "description": "test",
                   "region": region, "businessWorkspaceMode": "per-agent", "businessDatabaseName": "aidb",
                   "openvikingMode": "auto"}
        result = await engine.dispatch("create", request)
        assert result["taskId"] == "test-task"
        selected_profile, cloud = check.call_args.args[:2]
        assert selected_profile.provider == cloud.provider == provider
        assert cloud.region == region
        payload = tasks.start.call_args.args[1]
        assert payload.get("provider", "volcengine") == provider
        assert tasks.start.call_args.kwargs["name_scope"] == {"provider": provider, "account": "test-account", "region": region}
        deletion_cloud = engine.cloud_factory(region)
        assert deletion_cloud.provider == provider and deletion_cloud.network.provider == provider
        with pytest.raises(LifecycleError):
            await engine.dispatch("previewDelete", {"runtimeId": "r-one", "region": "cn-beijing" if provider == "byteplus" else "ap-southeast-1"})
    finally:
        await engine.close()


def test_byteplus_profile_includes_worker_database_memory_and_observability_defaults():
    profile = load_studio_profile(region="ap-southeast-1", provider="byteplus")
    assert profile.provider == "byteplus"
    assert profile.managed.postgres.mode == "auto"
    assert profile.openviking_auto_available
    runtime = profile.managed.runtime.env
    worker = profile.managed.worker.env
    for values in (runtime, worker):
        assert values["CLOUD_PROVIDER"] == "byteplus"
        assert values["BYTEPLUS_REGION"] == "ap-southeast-1"
    assert runtime["APIG_TOP_ENDPOINT"] == "apig.ap-southeast-1.byteplusapi.com"
    assert "bytepluses.com" in runtime["OBSERVABILITY_OPENTELEMETRY_APMPLUS_ENDPOINT"]
    assert runtime["MPA_CODEX_WORKER_DEFAULT_MODEL"] == runtime["MPA_SELECTABLE_MODELS"]
    assert not runtime["MPA_SELECTABLE_MODELS"].startswith("doubao-")

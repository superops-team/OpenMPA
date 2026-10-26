import importlib.util
import subprocess
import tempfile
import unittest
from pathlib import Path

spec = importlib.util.spec_from_file_location("guard", Path(__file__).with_name("check_staged_secrets.py"))
guard = importlib.util.module_from_spec(spec)
spec.loader.exec_module(guard)


class StagedSecretGuardTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.repo = Path(self.temp.name)
        subprocess.run(["git", "init", "-q", str(self.repo)], check=True)

    def stage(self, name, text):
        p = self.repo / name
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(text)
        subprocess.run(["git", "-C", str(self.repo), "add", "--", name], check=True)

    def test_environment_file_is_rejected_even_when_empty(self):
        self.stage(".env.production", "")
        self.assertEqual(guard.evaluate(self.repo)[0]["rule"], "private-or-environment-file")

    def test_scans_index_instead_of_later_worktree_edits(self):
        fixture = "gh" + "p_" + "A1b2C3d4E5f6G7h8I9j0K1l2M3n4O5p6Q7r8"
        self.stage("config.txt", fixture)
        (self.repo / "config.txt").write_text("clean worktree")
        self.assertTrue(any(f["rule"] == "github-token" for f in guard.evaluate(self.repo)))

    def test_source_with_empty_credentials_and_variable_names_passes(self):
        self.stage("config.ts", "const secretKey = ''; const name = 'VOLCENGINE_SECRET_KEY';")
        self.assertEqual(guard.evaluate(self.repo), [])

    def test_diagnostics_do_not_expose_private_key_contents(self):
        self.stage("oops.txt", "-----BEGIN " + "PRIVATE KEY-----\nfixture-material")
        finding = guard.evaluate(self.repo)[0]
        self.assertEqual(set(finding), {"path", "rule"})


if __name__ == "__main__":
    unittest.main()

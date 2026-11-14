#!/usr/bin/env python3
"""Verify release archive contents and refusal to package dirty source."""
import hashlib
import pathlib
import shutil
import subprocess
import tarfile
import tempfile
import unittest


class PackageTests(unittest.TestCase):
    def test_verified_tree_and_dirty_refusal(self):
        source = pathlib.Path(__file__).with_name("package.py")
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            (root / "scripts").mkdir()
            shutil.copy2(source, root / "scripts/package.py")
            (root / "VERSION").write_text("v0.1.0\n")
            (root / "go.mod").write_text("module github.com/gatework/motor-core\n")
            (root / "LICENSE").write_text("fixture license\n")
            def git(*args):
                return subprocess.check_output(["git", *args], cwd=root)
            git("init", "-b", "main")
            git("add", "VERSION", "go.mod", "LICENSE", "scripts/package.py")
            git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.com", "commit", "-m", "fixture")
            subprocess.run(["python3", "scripts/package.py"], cwd=root, check=True, capture_output=True)
            dist = root / "dist"
            self.assertEqual((dist / "REVISION").read_text().strip(), git("rev-parse", "HEAD").decode().strip())
            for line in (dist / "SHA256SUMS").read_text().splitlines():
                digest, name = line.split("  ")
                self.assertEqual(hashlib.sha256((dist / name).read_bytes()).hexdigest(), digest)
            with tarfile.open(dist / "motor-core-v0.1.0.tar.gz") as archive:
                self.assertEqual(archive.extractfile("motor-core-v0.1.0/LICENSE").read(), b"fixture license\n")
            (root / "go.mod").write_text("modified\n")
            result = subprocess.run(["python3", "scripts/package.py"], cwd=root, capture_output=True)
            self.assertNotEqual(result.returncode, 0)


if __name__ == "__main__":
    unittest.main()

#!/usr/bin/env python3
"""Exercise exact-artifact publication with a local GitHub protocol fake."""
import hashlib
import json
import os
import pathlib
import shutil
import subprocess
import tempfile
import unittest
import zipfile

FAKE = r'''#!/usr/bin/env python3
import hashlib,json,os,pathlib,shutil,sys
base=pathlib.Path(os.environ['FAKE_STATE'])
args=sys.argv[1:]
def output(value): print(json.dumps(value))
if args[0]=='api':
 path=args[1]
 if 'workflows/ci.yml/runs?' in path:
  output({'workflow_runs':[] if os.environ.get('NO_CI') else [{'id':1,'head_sha':os.environ['SHA'],'event':'push','html_url':'https://ci.example/run/1'}]})
 elif '/runs/1/artifacts?' in path:
  output({'artifacts':[{'id':2,'name':'source-'+os.environ['SHA'],'expired':False,'digest':'sha256:'+hashlib.sha256((base/'artifact.zip').read_bytes()).hexdigest()}]})
 elif path.endswith('/artifacts/2/zip'):
  sys.stdout.buffer.write((base/'artifact.zip').read_bytes())
 elif '/releases?' in path:
  output([json.loads((base/'release.json').read_text())] if (base/'release.json').exists() else [])
 elif path.endswith('/releases'):
  body=json.load(sys.stdin);body['assets']=[];(base/'release.json').write_text(json.dumps(body));output(body)
 elif '/releases/tags/' in path:
  body=json.loads((base/'release.json').read_text());body['html_url']='https://example.test/release';output(body)
 else:raise SystemExit('unexpected API '+path)
else:
 action=args[1]
 if action=='upload':
  (base/'assets').mkdir(exist_ok=True)
  for name in args[3:]:
   p=pathlib.Path(name)
   if p.is_file():shutil.copy2(p,base/'assets'/p.name)
 elif action=='download':
  out=pathlib.Path(args[args.index('--dir')+1]);shutil.copytree(base/'assets',out)
 elif action=='edit':
  body=json.loads((base/'release.json').read_text());body['draft']=False;(base/'release.json').write_text(json.dumps(body))
 else:raise SystemExit('unexpected release action '+action)
'''


class ReleaseTests(unittest.TestCase):
    def test_publish_exact_ci_and_verify_rerun(self):
        scripts = pathlib.Path(__file__).parent
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            (root / 'scripts').mkdir()
            for name in ['release.py', 'package.py']:
                shutil.copy2(scripts / name, root / 'scripts' / name)
            (root / 'VERSION').write_text('v0.1.0\n')
            (root / 'CHANGELOG.md').write_text('# Changelog\n\n## v0.1.0\n\nInitial release.\n')
            (root / 'go.mod').write_text('module github.com/gatework/motor-core\n')
            def git(*args):
                return subprocess.check_output(['git', *args], cwd=root)
            git('init', '-b', 'main')
            git('add', 'scripts/release.py', 'scripts/package.py', 'VERSION', 'CHANGELOG.md', 'go.mod')
            git('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.com', 'commit', '-m', 'fixture')
            git('tag', 'v0.1.0')
            subprocess.run(['python3', 'scripts/package.py'], cwd=root, capture_output=True, check=True)
            state = root / 'fake'
            state.mkdir()
            with zipfile.ZipFile(state / 'artifact.zip', 'w') as archive:
                for p in (root / 'dist').iterdir():
                    archive.write(p, p.name)
            binary = root / 'bin'
            binary.mkdir()
            (binary / 'gh').write_text(FAKE)
            (binary / 'gh').chmod(0o755)
            env = dict(os.environ, PATH=str(binary)+':'+os.environ['PATH'], FAKE_STATE=str(state),
                       SHA=git('rev-parse', 'HEAD').decode().strip(), GITHUB_REPOSITORY='example/motor-core', RELEASE_TAG='v0.1.0')
            def release(**extra):
                return subprocess.run(['python3', 'scripts/release.py'], cwd=root, env=dict(env, **extra), capture_output=True, text=True)
            rejected = release(NO_CI='1')
            self.assertNotEqual(rejected.returncode, 0)
            self.assertFalse((state / 'release.json').exists())
            first = release()
            self.assertEqual(first.returncode, 0, first.stderr)
            self.assertFalse(json.loads((state / 'release.json').read_text())['draft'])
            rerun = release()
            self.assertEqual(rerun.returncode, 0, rerun.stderr)
            (state / 'assets' / 'REVISION').write_text('tampered\n')
            mismatch = release()
            self.assertNotEqual(mismatch.returncode, 0)
            self.assertIn('differs from CI', mismatch.stderr)


if __name__ == '__main__':
    unittest.main()

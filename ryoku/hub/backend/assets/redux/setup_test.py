import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

SPEC=importlib.util.spec_from_file_location('redux_setup',Path(__file__).with_name('setup.py'))
m=importlib.util.module_from_spec(SPEC);SPEC.loader.exec_module(m)

class InstallerTests(unittest.TestCase):
 def setUp(self):
  self.temp=tempfile.TemporaryDirectory();self.addCleanup(self.temp.cleanup)
  self.root=Path(self.temp.name); self.stage=self.root/'stage';self.stage.mkdir()
  self.env=patch.dict(os.environ,{'HOME':str(self.root/'home'),'XDG_DATA_HOME':str(self.root/'data'),'XDG_CONFIG_HOME':str(self.root/'config')})
  self.env.start();self.addCleanup(self.env.stop)
  self.paths=m.layout()
 def fake_run(self,*args):
  if args[:2]==('git','init'):
   p=self.stage/'native';(p/'third_party/ggml').mkdir(parents=True);(p/'LICENSE').write_text('native license');(p/'third_party/ggml/LICENSE').write_text('ggml license')
  if args[:2]==('cmake','--build'):
   p=self.stage/'build/libparakeet.so';p.parent.mkdir(parents=True);p.write_bytes(b'library')
 def fake_download(self,url,target,sha):target.write_bytes(b'model')
 def install(self):
  with patch.object(m,'run',self.fake_run),patch.object(m,'download',self.fake_download),patch.object(m.shutil,'which',return_value='/bin/tool'),patch.object(m.ctypes,'CDLL') as lib:
   lib.return_value.parakeet_capi_abi_version.return_value=10
   m.install(self.paths,self.stage)
 def test_install_remove_and_reinstall(self):
  unrelated=self.root/'data/voxtype/models/keep.gguf';unrelated.parent.mkdir(parents=True);unrelated.write_text('keep')
  self.install();self.assertTrue(all(p.is_file() for p in self.paths.values()))
  self.assertIn('--pool-threads 4 --chunk-seconds 30',self.paths['unit'].read_text())
  m.remove(self.paths);self.assertFalse(any(p.exists() for p in self.paths.values()));self.assertTrue(unrelated.exists())
  m.shutil.rmtree(self.stage/'build');m.shutil.rmtree(self.stage/'native')
  self.install();self.assertTrue(self.paths['receipt'].exists())
 def test_modified_files_are_not_deleted(self):
  self.install();self.paths['client'].write_text('user edited')
  with self.assertRaisesRegex(RuntimeError,'changed'):m.remove(self.paths)
  self.assertTrue(self.paths['model'].exists());self.assertTrue(self.paths['receipt'].exists())
 def test_interrupted_download_publishes_nothing(self):
  def cancel(*args):raise KeyboardInterrupt()
  with patch.object(m,'run',self.fake_run),patch.object(m,'download',cancel),patch.object(m.shutil,'which',return_value='/bin/tool'),patch.object(m.ctypes,'CDLL') as lib:
   lib.return_value.parakeet_capi_abi_version.return_value=10
   with self.assertRaises(KeyboardInterrupt):m.install(self.paths,self.stage)
  self.assertFalse(any(p.exists() for p in self.paths.values()))
 def test_publish_failure_rolls_back(self):
  real_link=os.link;count=0
  def fail(src,dst):
   nonlocal count
   count+=1
   if count==3:raise OSError('disk full')
   return real_link(src,dst)
  with patch.object(m.os,'link',fail):
   with self.assertRaisesRegex(OSError,'disk full'):self.install()
  self.assertFalse(any(p.exists() for p in self.paths.values()))
  self.assertFalse(list(self.root.rglob('*.installing')))
 def test_unrecognized_receipt_cannot_delete_other_files(self):
  self.paths['receipt'].parent.mkdir(parents=True)
  external=self.root/'keep';external.write_text('keep')
  self.paths['receipt'].write_text(json.dumps({'owner':m.OWNER,'files':{str(external):m.digest(external)}}))
  with self.assertRaisesRegex(RuntimeError,'outside'):m.remove(self.paths)
  self.assertTrue(external.exists())

if __name__=='__main__':unittest.main()

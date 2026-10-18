#!/usr/bin/env python3
"""Offscreen Dictation cancellation integration test. Set RYOKU_TEST_HUB_BIN."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
BINARY = os.environ.get('RYOKU_TEST_HUB_BIN')
PAGE = (ROOT/'ryoku/hub/quickshell/pages/DictationPage.qml').as_uri()


@unittest.skipUnless(BINARY and shutil.which('qs'), 'needs qs and RYOKU_TEST_HUB_BIN')
class DictationCancellation(unittest.TestCase):
    def scenario(self, immediate, install_only=False):
        with tempfile.TemporaryDirectory(prefix='ryoku-dictation-') as directory:
            root = Path(directory); bin_dir = root/'bin'; bin_dir.mkdir()
            snapshot = {'presets': [
                {'key':'whisper-fast','label':'Base English','provider':'Whisper','group':'english','present':True,'cloud':False,'detail':'test','keyKind':''},
                {'key':'whisper-small-en','label':'Small English','provider':'Whisper','group':'english','present':False,'cloud':False,'detail':'test','keyKind':''},
                {'key':'parakeet-redux','label':'Redux Multilingual + Adapter','provider':'Parakeet','group':'multilingual','present':False,'cloud':False,'detail':'test','keyKind':''}],
                'selected':'whisper-fast','enabled':True,'installed':True,'packageAvailable':True}
            backend = bin_dir/'ryoku-hub'
            backend.write_text('#!'+sys.executable+'\nimport sys,os\n'
                               'if sys.argv[2]=="get": print('+repr(json.dumps(snapshot))+')\n'
                               'else: os.execv('+repr(BINARY)+',['+repr(BINARY)+']+sys.argv[1:])\n')
            if install_only:
                backend.write_text('#!'+sys.executable+'\nimport sys\n'
                                   'if sys.argv[2]=="get": print('+repr(json.dumps(snapshot))+')\n')
            backend.chmod(0o755)
            voxtype = bin_dir/'voxtype'
            voxtype.write_text('#!/bin/sh\nmkdir -p "$XDG_DATA_HOME/voxtype/models"\n'
                               'printf partial > "$XDG_DATA_HOME/voxtype/models/.partial"\nsleep 30\n')
            voxtype.chmod(0o755)
            qml = root/'test.qml'
            qml.write_text('''import QtQuick
import Quickshell
ShellRoot {
 id: root
 property var page
 property int stage: 0
 property int ticks: 0
 Timer {
  interval: 200; running: true; repeat: true
  onTriggered: {
   root.ticks++;
   if(root.ticks>50) { console.log("FAIL timeout"); Qt.quit(); }
   if(!root.page) {
    var c=Qt.createComponent(__PAGE__);
    if(c.status===Component.Error) {console.log("FAIL "+c.errorString());Qt.quit();return;}
    root.page=c.createObject(root,{width:1100,height:850});
   }
   if(root.stage===0 && root.page.loaded) {
    root.page.download(__INSTALL_ONLY__ ? "parakeet-redux" : "whisper-small-en", !__INSTALL_ONLY__);root.stage=1;
    if(__INSTALL_ONLY__) root.stage=3;
    if(__IMMEDIATE__) {root.page.cancelDownload();root.stage=2;}
   }
   else if(root.stage===3 && root.page.downloading==="") {
    if(root.page.selected!=="whisper-fast" || root.page.notice.indexOf("Adapter installed")<0) console.log("FAIL package install selection");
    else console.log("PASS QML cancel");
    Qt.quit();
   }
   else if(root.stage===1) {root.page.cancelDownload();root.stage=2;}
   else if(root.stage===2 && root.page.downloading==="") {
    if(root.page.selected!=="whisper-fast" || root.page.notice.indexOf("canceled")<0) console.log("FAIL cancellation state");
    else console.log("PASS QML cancel");
    Qt.quit();
   }
  }
 }
}
'''.replace('__PAGE__',json.dumps(PAGE)).replace('__IMMEDIATE__',str(immediate).lower()).replace('__INSTALL_ONLY__',str(install_only).lower()))
            env = dict(os.environ, QT_QPA_PLATFORM='offscreen', PATH=str(bin_dir)+':'+os.environ['PATH'],
                       HOME=str(root), XDG_CONFIG_HOME=str(root/'config'), XDG_DATA_HOME=str(root/'data'),
                       XDG_CACHE_HOME=str(root/'cache'))
            for key in ['__QUICKSHELL_CRASH_INFO_FD','__QUICKSHELL_CRASH_DUMP_FD']:
                env.pop(key,None)
            result = subprocess.run(['qs','-p',str(qml)],env=env,capture_output=True,text=True,timeout=20)
            logs = result.stdout+result.stderr
            self.assertIn('PASS QML cancel',logs,logs)
            self.assertNotIn('FAIL ',logs,logs)
            self.assertFalse(list((root/'data/voxtype/models').glob('.ryoku-model-install-*')))
            self.assertFalse((root/'data/voxtype/models/ggml-small.en.bin').exists())

    def test_package_install_does_not_select_model(self):
        self.scenario(False, install_only=True)

    def test_cancel_running_install(self):
        self.scenario(False)

    def test_cancel_before_process_start(self):
        self.scenario(True)


if __name__=='__main__':
    unittest.main()

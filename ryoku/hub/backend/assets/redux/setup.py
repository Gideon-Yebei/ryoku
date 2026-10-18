#!/usr/bin/env python3
"""Transactional user installation for the Hub's Redux adapter option."""
import argparse
import ctypes
import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import sys
import urllib.request

NATIVE_REV = '5c9b0fbbd8e611d88e47d7e1ebae9bca9b447e8f'
MODEL_REV = '68f808a5903a0bf12657bca3a2a669d573abfb58'
MODEL_SHA = '574614b9a4d9f72ab202877a7ad6a1f4bf819dd1d27b9d42dfe8cd429fcebdd5'
MODEL_URL = f'https://huggingface.co/mudler/parakeet-cpp-gguf/resolve/{MODEL_REV}/redux-packed.gguf'
OWNER = 'ryoku-redux-v1'


def digest(path):
    h = hashlib.sha256()
    with path.open('rb') as stream:
        for block in iter(lambda: stream.read(1024*1024), b''):
            h.update(block)
    return h.hexdigest()


def layout():
    home = Path.home()
    config = Path(os.environ.get('XDG_CONFIG_HOME', home/'.config'))
    data = Path(os.environ.get('XDG_DATA_HOME', home/'.local/share'))
    root = data/'voxtype-redux'
    return {'model': data/'voxtype/models/redux-packed.gguf',
            'client': home/'.local/bin/voxtype-redux',
            'worker': root/'adapter.py', 'library': root/'runtime/libparakeet.so',
            'license': root/'NOTICE.md',
            'unit': config/'systemd/user/voxtype-redux.service',
            'receipt': root/'hub-install.json'}


def read_receipt(paths):
    doc = json.loads(paths['receipt'].read_text())
    if doc.get('owner') != OWNER or not isinstance(doc.get('files'), dict):
        raise RuntimeError('Unrecognized Redux installation receipt')
    allowed = {str(p) for key,p in paths.items() if key != 'receipt'}
    allowed.add(str(paths['unit'].parents[2]/'voxtype/redux-adapter.toml'))
    if not set(doc['files']).issubset(allowed):
        raise RuntimeError('Receipt contains files outside the Redux installation')
    return doc


def validate_remove(paths):
    doc = read_receipt(paths)
    for name, expected in doc['files'].items():
        path = Path(name)
        if path.is_symlink() or (path.exists() and digest(path) != expected):
            raise RuntimeError(f'Redux file was changed; leaving installation intact: {path}')
    return doc


def adopt(paths):
    # Existing deployments retain external/shared native libraries. Only these
    # known adapter files and the selected model become managed by the Hub.
    if paths['receipt'].exists():
        return read_receipt(paths)
    names = ['model','client','worker','unit']
    if not all(paths[k].is_file() and not paths[k].is_symlink() for k in names):
        raise RuntimeError('Existing Redux installation is incomplete; cannot adopt it')
    if 'voxtype-redux' not in paths['unit'].read_text() or 'serve' not in paths['unit'].read_text():
        raise RuntimeError('Unrecognized Redux worker unit')
    adapter = paths['client'].read_text()
    if 'parakeet_capi' not in adapter or 'UnixStreamServer' not in adapter:
        raise RuntimeError('Unrecognized Redux adapter')
    doc = {'owner': OWNER, 'adopted': True,
           'files': {str(paths[k]):digest(paths[k]) for k in names}}
    profile = paths['unit'].parents[2]/'voxtype/redux-adapter.toml'
    if profile.is_file() and not profile.is_symlink():
        text = profile.read_text()
        if 'voxtype-redux' in text and 'redux-packed.gguf' in text:
            doc['files'][str(profile)] = digest(profile)
    tmp = paths['receipt'].with_suffix('.adopting')
    try:
        with tmp.open('x') as stream: json.dump(doc,stream,indent=2)
        tmp.chmod(0o600)
        os.link(tmp,paths['receipt'])
    finally:
        tmp.unlink(missing_ok=True)
    return doc


def remove(paths):
    doc = validate_remove(paths)
    for name in doc['files']:
        Path(name).unlink(missing_ok=True)
    paths['receipt'].unlink(missing_ok=True)
    for key in ['library','worker']:
        try: paths[key].parent.rmdir()
        except OSError: pass


def run(*args):
    subprocess.run(list(map(str,args)),check=True,stdout=sys.stderr,stderr=sys.stderr)


def download(url, target, expected):
    with urllib.request.urlopen(url, timeout=30) as response, target.open('wb') as stream:
        total = int(response.headers.get('Content-Length',0)); count = 0; last = -1
        while block := response.read(1024*1024):
            stream.write(block); count += len(block)
            percent = min(95,int(count/total*95)) if total else 0
            if percent != last:
                print(f'{percent}%',flush=True); last = percent
    if digest(target) != expected:
        raise RuntimeError('Redux model checksum mismatch; nothing was installed')


def install(paths, stage):
    for tool in ['git','cmake','c++','make']:
        if shutil.which(tool) is None:
            raise RuntimeError(f'Redux installation needs {tool}; install the native build tools first')
    if sys.version_info < (3,11):
        raise RuntimeError('Redux needs Python 3.11 or newer')
    if any(p.exists() or p.is_symlink() for p in paths.values()):
        raise RuntimeError('Redux installation files already exist; remove them through Dictation before reinstalling')
    print('Building the parakeet.cpp adapter runtime...', file=sys.stderr, flush=True)
    source = stage/'native'
    run('git','init',source)
    run('git','-C',source,'fetch','--depth','1','https://github.com/mudler/parakeet.cpp.git',NATIVE_REV)
    run('git','-C',source,'checkout','--detach','FETCH_HEAD')
    run('git','-C',source,'submodule','update','--init','--depth','1','third_party/ggml')
    build = stage/'build'
    run('cmake','-S',source,'-B',build,'-DCMAKE_BUILD_TYPE=Release',
        '-DPARAKEET_SHARED=ON','-DBUILD_SHARED_LIBS=OFF','-DCMAKE_POSITION_INDEPENDENT_CODE=ON',
        '-DPARAKEET_BUILD_CLI=OFF','-DPARAKEET_BUILD_SERVER=OFF',
        '-DPARAKEET_WITH_CED=OFF','-DPARAKEET_WITH_VOICEDETECT=OFF')
    run('cmake','--build',build,'--target','parakeet','-j','2')
    library = build/'libparakeet.so'
    native = ctypes.CDLL(str(library))
    native.parakeet_capi_abi_version.restype = ctypes.c_int
    if native.parakeet_capi_abi_version() < 10 or not hasattr(native,'parakeet_capi_set_concurrency'):
        raise RuntimeError('Native runtime does not provide the required adapter ABI')
    model = stage/'redux-packed.gguf'
    download(MODEL_URL,model,MODEL_SHA)
    adapter = Path(__file__).with_name('adapter.py').read_bytes()
    # systemd expands percent and dollar even within quoted arguments.
    q = lambda x: json.dumps(str(x).replace('%','%%').replace('$','$$'),ensure_ascii=False)
    unit = f'''[Unit]
Description=Resident Redux adapter for stock Voxtype
[Service]
Type=notify
NotifyAccess=main
ExecStart={q(sys.executable)} {q(paths['worker'])} serve --backend cpp --model {q(paths['model'])} --library {q(paths['library'])} --pool-threads 4 --chunk-seconds 30
RuntimeDirectory=voxtype-redux
RuntimeDirectoryMode=0700
UMask=0077
Restart=on-failure
RestartSec=2
TimeoutStartSec=180
TimeoutStopSec=15
[Install]
WantedBy=default.target
'''
    contents = {'model':model,'library':library,'client':adapter,'worker':adapter,
                'unit':unit.encode(), 'license':(Path(__file__).with_name('UPSTREAM.md').read_bytes()+b'\n'+Path(__file__).with_name('LICENSE').read_bytes()+b'\n'+(source/'LICENSE').read_bytes()+b'\n'+(source/'third_party/ggml/LICENSE').read_bytes())}
    installed, prepared = [], []
    receipt_tmp = paths['receipt'].with_suffix('.tmp')
    try:
        hashes = {}
        for key, value in contents.items():
            dest = paths[key]; dest.parent.mkdir(parents=True,exist_ok=True)
            tmp = dest.with_name(dest.name+'.installing')
            # Exclusive creation never overwrites user files or follows symlinks.
            with tmp.open('xb') as stream:
                prepared.append(tmp)
                if isinstance(value,Path):
                    with value.open('rb') as src: shutil.copyfileobj(src,stream)
                else: stream.write(value)
            tmp.chmod(0o755 if key in ['client','worker','library'] else 0o600)
            hashes[str(dest)] = digest(tmp)
        receipt_tmp.write_text(json.dumps({'owner':OWNER,'files':hashes},indent=2)+'\n')
        receipt_tmp.chmod(0o600)
        # Publish only complete files. Blocking termination here protects the
        # few renames; all slow work above remains cancelable.
        signal.pthread_sigmask(signal.SIG_BLOCK,{signal.SIGTERM,signal.SIGINT})
        try:
            for key, tmp in zip(contents, prepared):
                dest = paths[key]
                os.link(tmp,dest)  # atomic, exclusive; never replace user files
                installed.append(dest)
            os.link(receipt_tmp, paths['receipt'])
        except BaseException:
            for path in reversed(installed): path.unlink(missing_ok=True)
            raise
        finally:
            signal.pthread_sigmask(signal.SIG_UNBLOCK,{signal.SIGTERM,signal.SIGINT})
    finally:
        for tmp in prepared: tmp.unlink(missing_ok=True)
        receipt_tmp.unlink(missing_ok=True)
    print('100%',flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action',choices=['install','adopt','check-remove','remove'])
    args = parser.parse_args()
    def cancel(signum,frame): raise KeyboardInterrupt('Installation canceled')
    signal.signal(signal.SIGTERM,cancel)
    paths = layout()
    if args.action == 'install': install(paths,Path(__file__).parent)
    elif args.action == 'adopt': adopt(paths)
    elif args.action == 'check-remove': validate_remove(paths)
    else: remove(paths)


if __name__ == '__main__':
    try: main()
    except (Exception,KeyboardInterrupt) as error:
        print(str(error) or 'Installation canceled',file=sys.stderr)
        sys.exit(1)

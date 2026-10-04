#!/usr/bin/env python3
"""Download the pinned official Element release and serve only on localhost."""
import argparse,hashlib,http.server,json,mimetypes,tarfile,urllib.request
from pathlib import Path
VERSION='1.12.30'
SHA256='41ecae1e7af5d09baf2c1a7646cebad51ced6a44257d2096b9618bff3fee625d'
URL=f'https://github.com/element-hq/element-web/releases/download/v{VERSION}/element-v{VERSION}.tar.gz'

def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('action',choices=['prepare','serve']);p.add_argument('--dir',required=True,type=Path);p.add_argument('--port',type=int,default=18098);p.add_argument('--matrix-port',type=int,default=18100);a=p.parse_args()
    target=a.dir/f'element-v{VERSION}'
    if a.action=='prepare':
        a.dir.mkdir(mode=0o700,parents=True,exist_ok=True)
        archive=a.dir/f'element-v{VERSION}.tar.gz'
        if not archive.exists():urllib.request.urlretrieve(URL,archive)
        if hashlib.sha256(archive.read_bytes()).hexdigest()!=SHA256:raise SystemExit('Element archive checksum mismatch')
        with tarfile.open(archive) as t:t.extractall(a.dir,filter='data')
        config={'default_server_config':{'m.homeserver':{'base_url':f'http://127.0.0.1:{a.matrix_port}','server_name':'agora.test'}},'disable_custom_urls':True,'disable_guests':True,'disable_3pid_login':True,'brand':'Agora acceptance','show_labs_settings':False,'features':{'feature_voip':False,'feature_widgets':False,'feature_location_sharing':False}}
        (target/'config.json').write_text(json.dumps(config));print('Element 1.12.30 archive checksum verified and local config prepared.');return
    mimetypes.add_type('application/javascript','.mjs')
    class QuietHandler(http.server.SimpleHTTPRequestHandler):
        def __init__(self,*args,**kwargs):super().__init__(*args,directory=str(target),**kwargs)
        def log_message(self,*args):pass
    print(f'Element 1.12.30 serving on http://127.0.0.1:{a.port}',flush=True)
    http.server.ThreadingHTTPServer(('127.0.0.1',a.port),QuietHandler).serve_forever()
if __name__=='__main__':main()

"""Disposable HTTPS source with synthetic official-format CSV, never a live VPN."""
import base64
import csv
from http.server import BaseHTTPRequestHandler, HTTPServer
import io
from pathlib import Path
import ssl
import subprocess

assert Path('/.dockerenv').exists()
root = Path('/tmp/xnv-ci-source')
root.mkdir(mode=0o700, exist_ok=True)
subprocess.run(['openssl', 'req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '1',
                '-subj', '/CN=127.0.0.1', '-addext', 'subjectAltName=IP:127.0.0.1',
                '-addext', 'basicConstraints=critical,CA:TRUE',
                '-keyout', str(root/'key.pem'), '-out', str(root/'cert.pem')],
               check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
(root/'key.pem').chmod(0o600)
csv_body = io.StringIO()
csv_body.write('*vpn_servers\n')
writer = csv.writer(csv_body)
writer.writerow(['#HostName', 'IP', 'Score', 'Ping', 'CountryShort', 'OpenVPN_ConfigData_Base64'])
profile = b'client\ndev tun\nproto tcp\nremote 8.8.8.8 1194\n'
writer.writerow(['ci-synthetic.example.invalid', '8.8.8.8', '100', '10', 'US', base64.b64encode(profile).decode()])
csv_body.write('*\n')
body = csv_body.getvalue().encode()

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header('Content-Type', 'text/csv')
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *_):
        pass

server = HTTPServer(('127.0.0.1', 0), Handler)
context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
context.load_cert_chain(root/'cert.pem', root/'key.pem')
server.socket = context.wrap_socket(server.socket, server_side=True)
(root/'port').write_text(str(server.server_port))
server.serve_forever()

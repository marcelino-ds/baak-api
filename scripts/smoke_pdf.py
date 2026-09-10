"""Exercise the embedded PDF worker in the production image using a local source."""

import hashlib
import json
import os
import socket
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import pdfplumber


def fixture_pdf():
    stream = b"BT /F1 12 Tf 50 750 Td (BAAK PDF smoke test) Tj ET"
    objects = [
        b"<< /Type /Catalog /Pages 2 0 R >>",
        b"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
        b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] "
        b"/Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
        b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
        b"<< /Length %d >>\nstream\n" % len(stream) + stream + b"\nendstream",
    ]
    data = bytearray(b"%PDF-1.4\n")
    offsets = [0]
    for number, obj in enumerate(objects, 1):
        offsets.append(len(data))
        data.extend(b"%d 0 obj\n" % number + obj + b"\nendobj\n")
    xref = len(data)
    data.extend(b"xref\n0 %d\n0000000000 65535 f \n" % len(offsets))
    for offset in offsets[1:]:
        data.extend(b"%010d 00000 n \n" % offset)
    data.extend(b"trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n" % (len(offsets), xref))
    return bytes(data)


def main():
    assert pdfplumber.__version__ == "0.11.9", pdfplumber.__version__
    pdf = fixture_pdf()
    calls = {"catalog": 0, "pdf": 0}

    class Source(BaseHTTPRequestHandler):
        def do_GET(self):
            if self.path == "/buku_pedoman":
                calls["catalog"] += 1
                body = b'<main><h3>Pedoman</h3><a href="/file/pedoman.pdf">Pedoman</a></main>'
                content_type = "text/html"
            elif self.path == "/file/pedoman.pdf":
                calls["pdf"] += 1
                body, content_type = pdf, "application/pdf"
            else:
                self.send_error(404)
                return
            self.send_response(200)
            self.send_header("Content-Type", content_type)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, *args):
            pass

    source = ThreadingHTTPServer(("127.0.0.1", 0), Source)
    thread = threading.Thread(target=source.serve_forever, daemon=True)
    thread.start()
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        port = listener.getsockname()[1]
    source_url = "http://127.0.0.1:%d" % source.server_port
    api_url = "http://127.0.0.1:%d" % port
    env = dict(os.environ, BASE_URL=source_url, PORT=str(port), CACHE_ENABLED="true", FLARESOLVERR_URL="")
    # The fixture and API communicate only over loopback.
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))

    def get(path):
        with opener.open(api_url + path, timeout=25) as response:
            assert response.headers["Cache-Control"] == "no-store"
            result = json.load(response)
            assert result["success"], result
            return result["data"]

    with tempfile.TemporaryFile() as logs:
        process = subprocess.Popen(["/usr/local/bin/baak-api"], env=env, stdout=logs, stderr=logs)
        try:
            deadline = time.monotonic() + 10
            while True:
                try:
                    get("/live")
                    break
                except urllib.error.URLError:
                    if process.poll() is not None or time.monotonic() > deadline:
                        raise
                    time.sleep(0.1)
            catalog = get("/dokumen/buku-pedoman")
            link = catalog["links"][0]
            assert link["id"] == hashlib.sha256((source_url + "/file/pedoman.pdf").encode()).hexdigest()
            endpoint = "/dokumen/buku-pedoman/%s/teks" % link["id"]
            document = get(endpoint)
            assert document["id"] == link["id"]
            assert document["title"] == "Pedoman"
            assert document["page_count"] == 1
            assert document["pages"][0]["text"] == "BAAK PDF smoke test"
            assert document["metadata"]["content_hash"] == hashlib.sha256(pdf).hexdigest()
            assert get(endpoint) == document
            assert calls == {"catalog": 1, "pdf": 1}, calls
            print("PASS: production image catalog -> PDF -> embedded worker -> JSON; cache and pdfplumber 0.11.9")
        except BaseException:
            logs.seek(0)
            print(logs.read().decode(errors="replace"))
            raise
        finally:
            process.terminate()
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)
            source.shutdown()
            source.server_close()
            thread.join(timeout=5)


if __name__ == "__main__":
    main()

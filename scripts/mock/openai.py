"""A fake OpenAI-compatible server that logs the model of every request.

GET  /<prefix>/v1/models            lists a fixed set of models
POST /<prefix>/v1/chat/completions  answers "MOCK-OK", streamed or not

Each request is appended to LOG as "<prefix> <model>".
"""
import json, os, sys, time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

LOG = os.environ["MOCK_LOG"]
PORT = int(os.environ.get("MOCK_PORT", "18080"))


class H(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def prefix(self):
        return self.path.strip("/").split("/")[0]

    def do_GET(self):
        body = json.dumps({"object": "list", "data": [
            {"id": m, "object": "model", "owned_by": "mock"} for m in ("a-good", "a-bad", "b-one")
        ]}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        n = int(self.headers.get("Content-Length", 0))
        req = json.loads(self.rfile.read(n) or b"{}")
        model = req.get("model")
        with open(LOG, "a") as f:
            f.write(f"{self.prefix()} {model}\n")
        created = int(time.time())
        if req.get("stream"):
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.end_headers()
            for delta, finish in (({"role": "assistant", "content": "MOCK-OK"}, None), ({}, "stop")):
                chunk = {"id": "x", "object": "chat.completion.chunk", "created": created, "model": model,
                         "choices": [{"index": 0, "delta": delta, "finish_reason": finish}]}
                self.wfile.write(b"data: " + json.dumps(chunk).encode() + b"\n\n")
            usage = {"id": "x", "object": "chat.completion.chunk", "created": created, "model": model,
                     "choices": [], "usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}}
            self.wfile.write(b"data: " + json.dumps(usage).encode() + b"\n\n")
            self.wfile.write(b"data: [DONE]\n\n")
            return
        body = json.dumps({"id": "x", "object": "chat.completion", "created": created, "model": model,
                           "choices": [{"index": 0, "message": {"role": "assistant", "content": "MOCK-OK"},
                                        "finish_reason": "stop"}],
                           "usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


ThreadingHTTPServer(("127.0.0.1", PORT), H).serve_forever()

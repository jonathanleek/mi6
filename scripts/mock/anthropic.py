"""A fake Anthropic Messages API that logs the model of every request.

POST /v1/messages answers "MOCK-OK", streamed or not. Each request is
appended to LOG as "<path> <model>". Anything else gets an empty 200.
"""
import json, os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

LOG = os.environ["MOCK_LOG"]
PORT = int(os.environ.get("MOCK_PORT", "18081"))


def sse(w, event, data):
    w.write(f"event: {event}\ndata: {json.dumps(data)}\n\n".encode())


class H(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def reply(self, obj):
        body = json.dumps(obj).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        self.reply({"data": [], "has_more": False})

    def do_POST(self):
        n = int(self.headers.get("Content-Length", 0))
        raw = self.rfile.read(n) or b"{}"
        req = json.loads(raw)
        path = self.path.split("?")[0]
        model = req.get("model")
        with open(LOG, "a") as f:
            f.write(f"{path} {model}{' PINEAPPLE' if b'PINEAPPLE' in raw else ''}\n")
        if path.endswith("/count_tokens"):
            return self.reply({"input_tokens": 1})
        if not path.endswith("/messages"):
            return self.reply({})
        msg = {"id": "msg_mock", "type": "message", "role": "assistant", "model": model,
               "content": [], "stop_reason": None, "stop_sequence": None,
               "usage": {"input_tokens": 1, "output_tokens": 1}}
        if not req.get("stream"):
            msg["content"] = [{"type": "text", "text": "MOCK-OK"}]
            msg["stop_reason"] = "end_turn"
            return self.reply(msg)
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.end_headers()
        sse(self.wfile, "message_start", {"type": "message_start", "message": msg})
        sse(self.wfile, "content_block_start", {"type": "content_block_start", "index": 0,
                                                "content_block": {"type": "text", "text": ""}})
        sse(self.wfile, "content_block_delta", {"type": "content_block_delta", "index": 0,
                                                "delta": {"type": "text_delta", "text": "MOCK-OK"}})
        sse(self.wfile, "content_block_stop", {"type": "content_block_stop", "index": 0})
        sse(self.wfile, "message_delta", {"type": "message_delta",
                                          "delta": {"stop_reason": "end_turn", "stop_sequence": None},
                                          "usage": {"output_tokens": 1}})
        sse(self.wfile, "message_stop", {"type": "message_stop"})


ThreadingHTTPServer(("127.0.0.1", PORT), H).serve_forever()

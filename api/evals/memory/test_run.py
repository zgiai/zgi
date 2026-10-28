"""Tests for run.py against a fake external Agent API.

Run with: python -m unittest test_run.py
"""

import http.server
import json
import threading
import unittest
import urllib.parse

import run

CONVERSATION = "11111111-1111-1111-1111-111111111111"
MESSAGE = "22222222-2222-2222-2222-222222222222"


class FakeAgentAPI(http.server.BaseHTTPRequestHandler):
    """Asks a question on the first turn and, like the runtime, rejects a new
    chat turn on that conversation until the question is answered."""

    requests = []
    waiting = False

    def log_message(self, *args):
        pass

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        path = urllib.parse.urlparse(self.path)
        FakeAgentAPI.requests.append((path.path, urllib.parse.parse_qs(path.query), body))
        if path.path == "/api/v1/agents/chat":
            if body.get("conversation_id") == CONVERSATION and FakeAgentAPI.waiting:
                self._json(400, {"code": "199001", "message": "conversation is waiting for workflow question answer"})
                return
            FakeAgentAPI.waiting = True
            self._stream([
                ("message_start", {"conversation_id": CONVERSATION, "message_id": MESSAGE}),
                ("user_input_requested", {"conversation_id": CONVERSATION, "message_id": MESSAGE, "request_id": "call_1",
                                          "questions": [{"id": "city", "question": "Which city?"}, {"question": "When?"}]}),
                ("message_end", {"conversation_id": CONVERSATION, "message_id": MESSAGE, "status": "waiting_question"}),
            ])
        elif path.path == f"/api/v1/agents/conversations/{CONVERSATION}/messages/{MESSAGE}/user-input/call_1/continue":
            FakeAgentAPI.waiting = False
            self._stream([
                ("message", {"conversation_id": CONVERSATION, "message_id": MESSAGE, "answer": "Noted: Chengdu."}),
                ("message_end", {"conversation_id": CONVERSATION, "message_id": MESSAGE, "status": "completed"}),
            ])
        else:
            self._json(404, {"code": "404", "message": "not found"})

    def _json(self, status, payload):
        data = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def _stream(self, events):
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.end_headers()
        for kind, data in events:
            self.wfile.write(("data: " + json.dumps({"event": kind, "data": data}) + "\n\n").encode())


class RunScenarioTest(unittest.TestCase):
    def setUp(self):
        FakeAgentAPI.requests, FakeAgentAPI.waiting = [], False
        self.server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), FakeAgentAPI)
        threading.Thread(target=self.server.serve_forever, daemon=True).start()
        self.client = run.Client(f"http://127.0.0.1:{self.server.server_address[1]}")
        self.client.api_key = "test-key"

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()

    def test_answers_a_pending_question_instead_of_starting_a_new_turn(self):
        first = self.client.chat("eval-user", "I am choosing between Chengdu and Hangzhou.")
        self.assertEqual(first["pending_input"]["question_ids"], ["city", "q2"])

        second = self.client.chat("eval-user", "Chengdu, remember it.", first["conversation_id"], first["pending_input"])

        path, query, body = FakeAgentAPI.requests[-1]
        self.assertTrue(path.endswith("/user-input/call_1/continue"))
        self.assertEqual(query["user"], ["eval-user"])
        self.assertEqual(body, {"answers": {"city": "Chengdu, remember it.", "q2": "Chengdu, remember it."}})
        self.assertTrue(second["answered_question"])
        self.assertIsNone(second["pending_input"])
        self.assertEqual(second["answer"], "Noted: Chengdu.")

    def test_new_turn_on_a_waiting_conversation_reports_the_response_body(self):
        first = self.client.chat("eval-user", "I am choosing between Chengdu and Hangzhou.")
        with self.assertRaisesRegex(RuntimeError, "400 .*waiting for workflow question answer"):
            self.client.chat("eval-user", "Chengdu.", first["conversation_id"])


if __name__ == "__main__":
    unittest.main()

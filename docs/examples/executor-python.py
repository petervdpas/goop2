#!/usr/bin/env python3
"""
Goop2 Cluster Executor — Python example

Usage: python executor-python.py [--url http://localhost:8787]

Polls for jobs, claims them, processes, reports result.
Replace `process_job()` with your actual workload.
"""

import argparse
import os
import sys
import time
import threading
import requests


def process_job(job: dict, report_progress) -> dict:
    """
    Replace this with your actual job processing logic.

    Args:
        job: The job dict with id, type, payload, timeout_s, etc.
        report_progress: Callable(percent, message, stats) to send updates.

    Returns:
        Result dict on success.

    Raises:
        Exception on failure.
    """
    job_type = job["type"]
    payload = job.get("payload", {})
    total_steps = payload.get("steps", 10)

    for step in range(1, total_steps + 1):
        time.sleep(0.5)  # simulate work
        pct = int(step / total_steps * 100)
        report_progress(pct, f"step {step}/{total_steps}", {
            "step": step,
            "memory_mb": os.getpid() % 1000,  # fake metric
        })

    return {"type": job_type, "steps_completed": total_steps}


class Executor:
    def __init__(self, base_url: str, poll_interval: float = 2.0):
        self.base = base_url.rstrip("/")
        self.poll_interval = poll_interval
        self.session = requests.Session()
        self.session.headers["Content-Type"] = "application/json"

    def get_jobs(self) -> dict:
        r = self.session.get(f"{self.base}/api/cluster/job")
        r.raise_for_status()
        return r.json()

    def accept(self, job_id: str) -> dict:
        r = self.session.post(f"{self.base}/api/cluster/accept", json={"job_id": job_id})
        r.raise_for_status()
        return r.json()

    def progress(self, job_id: str, percent: int, message: str = "", stats: dict = None):
        self.session.post(f"{self.base}/api/cluster/progress", json={
            "job_id": job_id,
            "percent": percent,
            "message": message,
            "stats": stats or {},
        })

    def result(self, job_id: str, success: bool, result: dict = None, error: str = ""):
        self.session.post(f"{self.base}/api/cluster/result", json={
            "job_id": job_id,
            "success": success,
            "result": result or {},
            "error": error,
        })

    def heartbeat(self, stats: dict = None):
        try:
            self.session.post(f"{self.base}/api/cluster/heartbeat", json={
                "stats": stats or {"executor": "python", "pid": os.getpid()},
            })
        except Exception:
            pass

    def run(self):
        # Start heartbeat thread
        def hb_loop():
            while True:
                self.heartbeat()
                time.sleep(10)

        t = threading.Thread(target=hb_loop, daemon=True)
        t.start()

        print(f"[executor] polling {self.base} for jobs...")
        while True:
            try:
                data = self.get_jobs()
                pending = data.get("pending", [])

                if not pending:
                    time.sleep(self.poll_interval)
                    continue

                pj = pending[0]
                job = pj["job"]
                job_id = job["id"]
                print(f"[executor] found job {job_id} (type={job['type']})")

                self.accept(job_id)
                print(f"[executor] accepted {job_id}")

                def report(pct, msg="", stats=None):
                    self.progress(job_id, pct, msg, stats)
                    print(f"[executor] progress: {pct}% {msg}")

                result = process_job(job, report)
                self.result(job_id, success=True, result=result)
                print(f"[executor] completed {job_id}")

            except requests.exceptions.ConnectionError:
                print("[executor] connection lost, retrying...")
                time.sleep(self.poll_interval)
            except Exception as e:
                # Report failure if we have a job_id
                if "job_id" in dir():
                    self.result(job_id, success=False, error=str(e))
                    print(f"[executor] failed {job_id}: {e}")
                else:
                    print(f"[executor] error: {e}")
                time.sleep(self.poll_interval)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Goop2 cluster executor")
    parser.add_argument("--url", default="http://localhost:8787", help="goop2 base URL")
    parser.add_argument("--poll", type=float, default=2.0, help="poll interval (seconds)")
    args = parser.parse_args()

    try:
        Executor(args.url, args.poll).run()
    except KeyboardInterrupt:
        print("\n[executor] stopped")
        sys.exit(0)

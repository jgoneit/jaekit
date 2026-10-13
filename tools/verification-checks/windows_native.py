#!/usr/bin/env python3
"""A pending real-Windows evidence gate. Never substitutes host-local checks.

AC-1 through AC-10 require the documented Windows observations. This gate
reports unobserved evidence until a versioned native acceptance consumer is
available; it does not manufacture a successful result from a cross-build.
"""
import json
import os
from pathlib import Path
import sys

criterion = sys.argv[1]
report = {"schema": "check-result/v1", "producer": "jaekit-windows-native", "producer_version": "1",
          "invocation": os.environ.get("HA_EVIDENCE_INVOCATION", ""),
          "declaration_digest": os.environ.get("HA_EVIDENCE_DECLARATION_DIGEST", ""),
          "attempts_complete": True,
          "observations": [{"target": criterion, "attempt": 1, "status": "skip", "reason": "not_selected"}]}
if os.environ.get("HA_EVIDENCE_PATH"):
    Path(os.environ["HA_EVIDENCE_PATH"]).write_text(json.dumps(report), encoding="utf-8")
print("Real Windows 11 x64 observation is pending for " + criterion, file=sys.stderr)
sys.exit(1)
